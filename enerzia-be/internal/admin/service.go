package admin

import (
	"context"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Sentinel errors returned by Login.
var (
	// ErrInvalidCredentials is returned for wrong email or wrong password.
	// The two cases are intentionally indistinguishable to callers.
	ErrInvalidCredentials = errors.New("admin: invalid credentials")
	// ErrNotConfigured is returned when no admin credentials have been set in
	// the environment. The handler maps it to the same 401 as ErrInvalidCredentials
	// so callers cannot distinguish "wrong password" from "no admin at all".
	ErrNotConfigured = errors.New("admin: sign-in not configured")
	// ErrRateLimited is returned when the IP has exceeded the failure limit.
	ErrRateLimited = errors.New("admin: rate limited")
)

// ServiceConfig holds the dependencies injected into a Service.
type ServiceConfig struct {
	Email        string
	PasswordHash string
	Tokens       *TokenIssuer
	Limiter      *Limiter
}

// Service implements the admin sign-in business logic.
type Service struct {
	email        string
	passwordHash string
	dummyHash    []byte // same cost as passwordHash for timing parity
	tokens       *TokenIssuer
	limiter      *Limiter
}

// NewService constructs a Service from cfg.
//
// The dummy hash is generated here at the same bcrypt cost as the configured
// hash, so that a wrong-email attempt takes the same wall-clock time as a
// wrong-password attempt. If the configured hash is malformed (bad paste in
// .env), bcrypt.Cost returns an error; the service treats itself as
// unconfigured rather than panicking or defaulting to a mismatched cost.
func NewService(cfg ServiceConfig) *Service {
	svc := &Service{
		email:        cfg.Email,
		passwordHash: cfg.PasswordHash,
		tokens:       cfg.Tokens,
		limiter:      cfg.Limiter,
	}

	if cfg.PasswordHash != "" {
		cost, err := bcrypt.Cost([]byte(cfg.PasswordHash))
		if err != nil {
			// A malformed hash is treated as "not configured" at runtime.
			// Zero out the fields so Login returns ErrNotConfigured.
			svc.email = ""
			svc.passwordHash = ""
		} else {
			dummy, err := bcrypt.GenerateFromPassword([]byte("dummy-sentinel-value"), cost)
			if err != nil {
				// Should not happen: cost was read from a valid hash.
				svc.email = ""
				svc.passwordHash = ""
			} else {
				svc.dummyHash = dummy
			}
		}
	}

	return svc
}

// Email returns the configured administrator email in its canonical form.
func (s *Service) Email() string { return s.email }

// ParseToken delegates token verification to the underlying TokenIssuer.
func (s *Service) ParseToken(token string) (Claims, error) {
	return s.tokens.Parse(token)
}

// Login authenticates with email and password from ip, and on success returns
// a signed admin token and its expiry.
//
// The limiter is checked first, even when the server is unconfigured, so the
// two states (no admin set up / wrong credentials) are indistinguishable from
// outside: both hit 429 after 10 failures from the same IP.
//
// Wrong email costs the same time as wrong password: when the email does not
// match, a bcrypt comparison is still performed against dummyHash (which has
// the same cost as the configured hash) so that response time reveals nothing
// about which email is configured.
func (s *Service) Login(_ context.Context, ip, email, password string) (token string, expiresAt time.Time, err error) {
	if !s.limiter.Allow(ip) {
		return "", time.Time{}, ErrRateLimited
	}

	if s.email == "" || s.passwordHash == "" {
		s.limiter.RecordFailure(ip)
		return "", time.Time{}, ErrNotConfigured
	}

	emailMatches := strings.EqualFold(email, s.email)

	// Always run a bcrypt comparison regardless of email match. When the email
	// is wrong we compare against dummyHash — a hash of a fixed sentinel with
	// the same cost as the real hash — so response time cannot tell an attacker
	// which email is configured.
	hashToCompare := []byte(s.passwordHash)
	if !emailMatches {
		hashToCompare = s.dummyHash
	}

	if bcrypt.CompareHashAndPassword(hashToCompare, []byte(password)) != nil || !emailMatches {
		s.limiter.RecordFailure(ip)
		return "", time.Time{}, ErrInvalidCredentials
	}

	tok, exp, err := s.tokens.Issue(s.email)
	if err != nil {
		return "", time.Time{}, err
	}
	return tok, exp, nil
}
