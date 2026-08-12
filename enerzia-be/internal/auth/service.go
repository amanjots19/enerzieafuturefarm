package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/enerzia/enerzia-be/internal/msg91"
)

// Service-level failures. Each maps to exactly one API response, so the
// handler never has to interpret a database error.
var (
	// ErrInvalidPhone means the number is not ten digits.
	ErrInvalidPhone = errors.New("auth: invalid phone number")
	// ErrInvalidCodeShape means the code is not six digits. It says nothing
	// about whether the code was right.
	ErrInvalidCodeShape = errors.New("auth: invalid code format")
	// ErrRateLimited means too many codes were requested too quickly.
	ErrRateLimited = errors.New("auth: too many code requests")
	// ErrCodeRejected covers wrong, expired, spent and locked-out codes.
	// Deliberately one error: telling a caller which of those it was would
	// help them enumerate.
	ErrCodeRejected = errors.New("auth: code rejected")
	// ErrSessionRejected means the MSG91 verifier rejected the access token,
	// or the phone number it returned was not normalisable to 10 digits.
	// One sentinel covers all rejection reasons so callers learn nothing
	// specific about what went wrong.
	ErrSessionRejected = errors.New("auth: session rejected")
	// ErrGatewayError means the MSG91 API was unreachable or returned an
	// unexpected status. Retrying may work.
	ErrGatewayError = errors.New("auth: gateway error")
)

// Store is the persistence surface the service needs. *Repository satisfies
// it; tests substitute a stub.
type Store interface {
	CreateCode(ctx context.Context, code OTPCode) error
	LatestCode(ctx context.Context, phone string) (OTPCode, error)
	CountCodesSince(ctx context.Context, phone string, since time.Time) (int64, error)
	RecordFailedAttempt(ctx context.Context, id bson.ObjectID) error
	ConsumeCode(ctx context.Context, id bson.ObjectID) (bool, error)
	UpsertUser(ctx context.Context, phone string, now time.Time) (User, error)
	UserByID(ctx context.Context, id bson.ObjectID) (User, error)
	SetAddresses(ctx context.Context, userID bson.ObjectID, addrs []Address, now time.Time) error
}

// Service holds the sign-in rules.
type Service struct {
	store        Store
	sender       Sender
	tokens       *TokenIssuer
	pepper       []byte
	revealCode   bool
	now          func() time.Time
	generateCode func() (string, error)
	verifier     msg91.Verifier
}

// ServiceConfig configures the sign-in service.
type ServiceConfig struct {
	Store  Store
	Sender Sender
	Tokens *TokenIssuer
	// Pepper keys the stored code hashes. Never logged, never returned.
	Pepper []byte
	// RevealCode puts the generated code in the response. Development only —
	// callers must pass false in production.
	RevealCode bool
	// Verifier checks MSG91 widget access tokens. Defaults to
	// msg91.Unconfigured{} when nil.
	Verifier msg91.Verifier
}

// NewService builds the sign-in service.
func NewService(cfg ServiceConfig) *Service {
	v := cfg.Verifier
	if v == nil {
		v = msg91.Unconfigured{}
	}
	return &Service{
		store:        cfg.Store,
		sender:       cfg.Sender,
		tokens:       cfg.Tokens,
		pepper:       cfg.Pepper,
		revealCode:   cfg.RevealCode,
		now:          time.Now,
		generateCode: GenerateCode,
		verifier:     v,
	}
}

// RequestResult describes an issued challenge.
type RequestResult struct {
	ExpiresIn   time.Duration
	ResendAfter time.Duration
	// DevCode is the plaintext code, present only outside production.
	DevCode string
}

// RequestCode issues a one-time code and hands it to the sender.
//
// The code is stored hashed and returned to the caller only in development.
// Rate limits are enforced before anything is generated, so a flood costs one
// count query rather than an SMS.
func (s *Service) RequestCode(ctx context.Context, phone string) (RequestResult, error) {
	if !ValidPhone(phone) {
		return RequestResult{}, ErrInvalidPhone
	}

	now := s.now()
	if err := s.checkRateLimit(ctx, phone, now); err != nil {
		return RequestResult{}, err
	}

	code, err := s.generateCode()
	if err != nil {
		return RequestResult{}, fmt.Errorf("auth: generate code: %w", err)
	}

	record := OTPCode{
		Phone:     phone,
		CodeHash:  HashCode(s.pepper, phone, code),
		CreatedAt: now,
		ExpiresAt: now.Add(CodeTTL),
	}
	if err := s.store.CreateCode(ctx, record); err != nil {
		return RequestResult{}, err
	}

	if err := s.sender.Send(ctx, phone, code); err != nil {
		return RequestResult{}, fmt.Errorf("auth: send code: %w", err)
	}

	result := RequestResult{ExpiresIn: CodeTTL, ResendAfter: ResendInterval}
	if s.revealCode {
		result.DevCode = code
	}
	return result, nil
}

// checkRateLimit enforces both the per-window cap and the resend interval.
func (s *Service) checkRateLimit(ctx context.Context, phone string, now time.Time) error {
	count, err := s.store.CountCodesSince(ctx, phone, now.Add(-RateWindow))
	if err != nil {
		return err
	}
	if count >= MaxRequestsPerWindow {
		return ErrRateLimited
	}

	latest, err := s.store.LatestCode(ctx, phone)
	switch {
	case errors.Is(err, ErrCodeNotFound):
		return nil // first request for this number
	case err != nil:
		return err
	}
	if now.Sub(latest.CreatedAt) < ResendInterval {
		return ErrRateLimited
	}
	return nil
}

// VerifyResult is a completed sign-in.
type VerifyResult struct {
	Token     string
	ExpiresAt time.Time
	User      User
}

// Verify checks a code and, on success, signs the shopper in — creating their
// account if this is their first time.
//
// This is what closes the frontend gap noted in product.md §3.3, where any six
// digits were accepted.
func (s *Service) Verify(ctx context.Context, phone, code string) (VerifyResult, error) {
	if !ValidPhone(phone) {
		return VerifyResult{}, ErrInvalidPhone
	}
	if !ValidCodeShape(code) {
		return VerifyResult{}, ErrInvalidCodeShape
	}

	now := s.now()

	record, err := s.store.LatestCode(ctx, phone)
	switch {
	case errors.Is(err, ErrCodeNotFound):
		return VerifyResult{}, ErrCodeRejected
	case err != nil:
		return VerifyResult{}, err
	}

	if !record.Usable(now) {
		return VerifyResult{}, ErrCodeRejected
	}

	if !CodeMatches(s.pepper, phone, code, record.CodeHash) {
		// Count the miss before returning, or the lockout never bites.
		if recordErr := s.store.RecordFailedAttempt(ctx, record.ID); recordErr != nil {
			return VerifyResult{}, recordErr
		}
		return VerifyResult{}, ErrCodeRejected
	}

	// Consuming before issuing a token closes the replay window: two requests
	// carrying the same correct code cannot both produce a session.
	consumed, err := s.store.ConsumeCode(ctx, record.ID)
	if err != nil {
		return VerifyResult{}, err
	}
	if !consumed {
		return VerifyResult{}, ErrCodeRejected
	}

	user, err := s.store.UpsertUser(ctx, phone, now)
	if err != nil {
		return VerifyResult{}, err
	}

	token, expiresAt, err := s.tokens.Issue(user)
	if err != nil {
		return VerifyResult{}, err
	}
	return VerifyResult{Token: token, ExpiresAt: expiresAt, User: user}, nil
}

// UserByID loads the signed-in shopper behind a token.
func (s *Service) UserByID(ctx context.Context, id bson.ObjectID) (User, error) {
	return s.store.UserByID(ctx, id)
}

// ParseToken verifies a bearer token. The middleware uses it.
func (s *Service) ParseToken(token string) (Claims, error) { return s.tokens.Parse(token) }

// CreateSession exchanges an MSG91 widget access token for a session JWT,
// creating the user on first sign-in. It is the server-side half of the
// MSG91 OTP widget flow defined in roadmap.md §Auth.
func (s *Service) CreateSession(ctx context.Context, accessToken string) (VerifyResult, error) {
	rawPhone, err := s.verifier.VerifyAccessToken(ctx, accessToken)
	if err != nil {
		switch {
		case errors.Is(err, msg91.ErrVerificationFailed), errors.Is(err, msg91.ErrNotConfigured):
			// Wrap rather than replace: errors.Is(result, ErrSessionRejected) stays true,
			// AND errors.As(result, *VerificationError) succeeds when MSG91 returned detail,
			// so the handler can log the code/message without returning them to the client.
			return VerifyResult{}, fmt.Errorf("%w: %w", ErrSessionRejected, err)
		default:
			// Covers unreachable service, context cancellation, etc.
			// Wrap to preserve the underlying cause in the handler's error log.
			return VerifyResult{}, fmt.Errorf("%w: %w", ErrGatewayError, err)
		}
	}

	phone, err := normalizePhone(rawPhone)
	if err != nil {
		// Not 10 digits after stripping the country code — reject rather than
		// store a malformed identity that would silently create a second account.
		return VerifyResult{}, ErrSessionRejected
	}

	now := s.now()
	user, err := s.store.UpsertUser(ctx, phone, now)
	if err != nil {
		return VerifyResult{}, fmt.Errorf("auth: create session: upsert: %w", err)
	}

	token, expiresAt, err := s.tokens.Issue(user)
	if err != nil {
		return VerifyResult{}, err
	}
	return VerifyResult{Token: token, ExpiresAt: expiresAt, User: user}, nil
}

// normalizePhone strips the Indian country code from a raw phone string and
// validates the result is exactly 10 digits.
//
// MSG91 returns the number with the country code, e.g. "919999999999". The
// users collection stores 10 digits ("9999999999"). A mismatch here would
// silently create two accounts for the same shopper.
func normalizePhone(raw string) (string, error) {
	s := raw
	if len(s) == 12 && s[:2] == "91" {
		s = s[2:]
	}
	if !ValidPhone(s) {
		return "", fmt.Errorf("auth: phone %q does not reduce to 10 digits", raw)
	}
	return s, nil
}
