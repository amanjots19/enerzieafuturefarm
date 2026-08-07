package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// issuerName identifies tokens minted by this service.
const issuerName = "enerzia-api"

// ErrInvalidToken covers every reason a bearer token is unacceptable —
// tampered, expired, wrong algorithm, wrong issuer. Callers get one error
// because the client should learn nothing about which check failed.
var ErrInvalidToken = errors.New("auth: invalid token")

// Claims is the payload carried in a bearer token.
type Claims struct {
	Phone string `json:"phone"`
	jwt.RegisteredClaims
}

// UserID returns the subject as an ObjectID.
func (c Claims) UserID() (bson.ObjectID, error) {
	id, err := bson.ObjectIDFromHex(c.Subject)
	if err != nil {
		return bson.NilObjectID, fmt.Errorf("%w: subject is not an object id", ErrInvalidToken)
	}
	return id, nil
}

// TokenIssuer mints and verifies bearer tokens.
type TokenIssuer struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time // injectable so expiry is testable without sleeping
}

// NewTokenIssuer builds an issuer over an HS256 secret.
func NewTokenIssuer(secret []byte, ttl time.Duration) *TokenIssuer {
	return NewTokenIssuerAt(secret, ttl, time.Now)
}

// NewTokenIssuerAt is NewTokenIssuer with an injectable clock, so expiry
// behaviour can be tested without sleeping.
func NewTokenIssuerAt(secret []byte, ttl time.Duration, now func() time.Time) *TokenIssuer {
	return &TokenIssuer{secret: secret, ttl: ttl, now: now}
}

// Issue returns a signed token for a user, and the moment it expires.
func (t *TokenIssuer) Issue(user User) (string, time.Time, error) {
	now := t.now()
	expiresAt := now.Add(t.ttl)

	claims := Claims{
		Phone: user.Phone,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.Hex(),
			Issuer:    issuerName,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth: sign token: %w", err)
	}
	return signed, expiresAt, nil
}

// Parse verifies a token and returns its claims.
//
// The signing method is pinned to HS256. Without that check a token could
// arrive with alg "none", or with an RSA header that tricks the library into
// treating the HMAC secret as a public key.
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
	if claims.Subject == "" {
		return Claims{}, ErrInvalidToken
	}
	return claims, nil
}
