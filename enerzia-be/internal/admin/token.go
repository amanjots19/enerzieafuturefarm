// Package admin provides administrator sign-in, token issuance, and HTTP
// middleware for the admin sub-API. It is intentionally separate from the
// customer-facing auth package so the two issuer namespaces cannot be confused.
package admin

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// issuerName identifies tokens minted by the admin package.
// Tokens with issuer "enerzia-api" (customer tokens) are rejected here, and
// admin tokens are rejected by the customer auth middleware — cross-issuer
// confusion is impossible.
const issuerName = "enerzia-admin"

// TokenTTL is the lifetime of an admin bearer token.
const TokenTTL = 12 * time.Hour

// ErrInvalidToken covers every reason an admin bearer token is unacceptable.
var ErrInvalidToken = errors.New("admin: invalid token")

// Claims is the payload carried in an admin bearer token.
type Claims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

// TokenIssuer mints and verifies admin bearer tokens.
type TokenIssuer struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

// NewTokenIssuer builds an issuer over an HS256 secret.
func NewTokenIssuer(secret []byte, ttl time.Duration) *TokenIssuer {
	return NewTokenIssuerAt(secret, ttl, time.Now)
}

// NewTokenIssuerAt is NewTokenIssuer with an injectable clock so expiry
// behaviour can be tested without sleeping.
func NewTokenIssuerAt(secret []byte, ttl time.Duration, now func() time.Time) *TokenIssuer {
	return &TokenIssuer{secret: secret, ttl: ttl, now: now}
}

// Issue returns a signed admin token for email, and the moment it expires.
func (t *TokenIssuer) Issue(email string) (string, time.Time, error) {
	now := t.now()
	expiresAt := now.Add(t.ttl)

	claims := Claims{
		Role: "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   email,
			Issuer:    issuerName,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("admin: sign token: %w", err)
	}
	return signed, expiresAt, nil
}

// Parse verifies an admin token and returns its claims.
//
// The signing method is pinned to HS256 to prevent alg-confusion attacks.
// Both the issuer ("enerzia-admin") and the role ("admin") must be present
// so a customer token cannot be replayed here even if signed with the same
// secret.
func (t *TokenIssuer) Parse(token string) (Claims, error) {
	var claims Claims

	parsed, err := jwt.ParseWithClaims(token, &claims,
		func(tok *jwt.Token) (any, error) {
			if _, ok := tok.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("%w: unexpected signing method %v", ErrInvalidToken, tok.Header["alg"])
			}
			return t.secret, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(issuerName),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(t.now),
	)
	if err != nil || !parsed.Valid {
		return Claims{}, ErrInvalidToken
	}
	if claims.Subject == "" || claims.Role != "admin" {
		return Claims{}, ErrInvalidToken
	}
	return claims, nil
}
