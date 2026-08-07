package auth_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/enerzia/enerzia-be/internal/auth"
)

var tokenSecret = []byte("token-secret-at-least-32-characters-long")

func testUser() auth.User {
	return auth.User{ID: bson.NewObjectID(), Phone: "9876543210"}
}

func TestIssueAndParseRoundTrip(t *testing.T) {
	issuer := auth.NewTokenIssuer(tokenSecret, auth.TokenTTL)
	user := testUser()

	token, expiresAt, err := issuer.Issue(user)
	if err != nil {
		t.Fatalf("Issue() error = %v, want nil", err)
	}
	if token == "" {
		t.Fatal("Issue() returned an empty token")
	}
	if d := time.Until(expiresAt); d < auth.TokenTTL-time.Minute || d > auth.TokenTTL+time.Minute {
		t.Errorf("expiresAt is %v away, want about %v", d, auth.TokenTTL)
	}

	claims, err := issuer.Parse(token)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if claims.Subject != user.ID.Hex() {
		t.Errorf("subject = %q, want %q", claims.Subject, user.ID.Hex())
	}
	if claims.Phone != user.Phone {
		t.Errorf("phone = %q, want %q", claims.Phone, user.Phone)
	}

	gotID, err := claims.UserID()
	if err != nil {
		t.Fatalf("UserID() error = %v", err)
	}
	if gotID != user.ID {
		t.Errorf("UserID() = %v, want %v", gotID, user.ID)
	}
}

func TestTokenDoesNotCarryTheSecret(t *testing.T) {
	issuer := auth.NewTokenIssuer(tokenSecret, auth.TokenTTL)
	token, _, err := issuer.Issue(testUser())
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if strings.Contains(token, string(tokenSecret)) {
		t.Error("the signing secret appears inside the token")
	}
}

func TestParseRejectsBadTokens(t *testing.T) {
	issuer := auth.NewTokenIssuer(tokenSecret, auth.TokenTTL)
	valid, _, err := issuer.Issue(testUser())
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	// A token signed with the right shape but the wrong key.
	otherIssuer := auth.NewTokenIssuer([]byte("a-completely-different-secret-key-32"), auth.TokenTTL)
	foreign, _, err := otherIssuer.Issue(testUser())
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	// alg:none, the classic JWT bypass.
	unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.RegisteredClaims{
		Subject:   bson.NewObjectID().Hex(),
		Issuer:    "enerzia-api",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("building the alg:none token: %v", err)
	}

	// A token from another service that happens to share our secret.
	wrongIssuer, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   bson.NewObjectID().Hex(),
		Issuer:    "someone-else",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}).SignedString(tokenSecret)
	if err != nil {
		t.Fatalf("building the wrong-issuer token: %v", err)
	}

	// A token with no expiry at all.
	noExpiry, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject: bson.NewObjectID().Hex(),
		Issuer:  "enerzia-api",
	}).SignedString(tokenSecret)
	if err != nil {
		t.Fatalf("building the no-expiry token: %v", err)
	}

	tests := []struct {
		name  string
		token string
	}{
		{name: "empty", token: ""},
		{name: "not a jwt", token: "hello"},
		{name: "truncated", token: valid[:len(valid)-6]},
		{name: "tampered payload", token: valid[:20] + "x" + valid[21:]},
		{name: "signed with another key", token: foreign},
		{name: "alg none", token: unsigned},
		{name: "wrong issuer", token: wrongIssuer},
		{name: "no expiry", token: noExpiry},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := issuer.Parse(tt.token); !errors.Is(err, auth.ErrInvalidToken) {
				t.Errorf("Parse() error = %v, want ErrInvalidToken", err)
			}
		})
	}
}

func TestParseRejectsAnExpiredToken(t *testing.T) {
	issuer := auth.NewTokenIssuer(tokenSecret, time.Hour)

	// Issue in the past so the token is already stale, without sleeping.
	past := auth.NewTokenIssuerAt(tokenSecret, time.Hour, func() time.Time {
		return time.Now().Add(-2 * time.Hour)
	})
	token, _, err := past.Issue(testUser())
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	if _, err := issuer.Parse(token); !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("Parse() error = %v, want ErrInvalidToken for an expired token", err)
	}
}

func TestParseAcceptsATokenIssuedMomentsAgo(t *testing.T) {
	issuer := auth.NewTokenIssuer(tokenSecret, time.Hour)
	token, _, err := issuer.Issue(testUser())
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if _, err := issuer.Parse(token); err != nil {
		t.Errorf("Parse() error = %v, want nil", err)
	}
}

func TestUserIDRejectsANonObjectIDSubject(t *testing.T) {
	claims := auth.Claims{}
	claims.Subject = "not-an-object-id"

	if _, err := claims.UserID(); !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("UserID() error = %v, want ErrInvalidToken", err)
	}
}

func TestParseRejectsAnEmptySubject(t *testing.T) {
	// A signed token with no subject names nobody.
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer:    "enerzia-api",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}).SignedString(tokenSecret)
	if err != nil {
		t.Fatalf("building the token: %v", err)
	}

	issuer := auth.NewTokenIssuer(tokenSecret, time.Hour)
	if _, err := issuer.Parse(token); !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("Parse() error = %v, want ErrInvalidToken", err)
	}
}
