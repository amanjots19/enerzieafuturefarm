package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
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
}

// NewService builds the sign-in service.
func NewService(cfg ServiceConfig) *Service {
	return &Service{
		store:        cfg.Store,
		sender:       cfg.Sender,
		tokens:       cfg.Tokens,
		pepper:       cfg.Pepper,
		revealCode:   cfg.RevealCode,
		now:          time.Now,
		generateCode: GenerateCode,
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
