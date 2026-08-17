package admin_test

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/enerzia/enerzia-be/internal/admin"
	"github.com/enerzia/enerzia-be/internal/auth"
)

var adminSecret = []byte("admin-token-secret-at-least-32-chars!!")

func TestAdminIssueAndParseRoundTrip(t *testing.T) {
	issuer := admin.NewTokenIssuer(adminSecret, admin.TokenTTL)

	token, expiresAt, err := issuer.Issue("ops@enerzia.in")
	if err != nil {
		t.Fatalf("Issue() error = %v, want nil", err)
	}
	if token == "" {
		t.Fatal("Issue() returned an empty token")
	}
	if d := time.Until(expiresAt); d < admin.TokenTTL-time.Minute || d > admin.TokenTTL+time.Minute {
		t.Errorf("expiresAt is %v away, want about %v", d, admin.TokenTTL)
	}

	claims, err := issuer.Parse(token)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if claims.Subject != "ops@enerzia.in" {
		t.Errorf("subject = %q, want %q", claims.Subject, "ops@enerzia.in")
	}
	if claims.Role != "admin" {
		t.Errorf("role = %q, want %q", claims.Role, "admin")
	}
}

func TestAdminParseRejectsBadTokens(t *testing.T) {
	issuer := admin.NewTokenIssuer(adminSecret, admin.TokenTTL)
	valid, _, err := issuer.Issue("ops@enerzia.in")
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	// Token signed with a different secret.
	otherIssuer := admin.NewTokenIssuer([]byte("a-completely-different-secret-key-32"), admin.TokenTTL)
	foreign, _, _ := otherIssuer.Issue("ops@enerzia.in")

	// alg:none bypass.
	unsigned, _ := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.RegisteredClaims{
		Subject:   "ops@enerzia.in",
		Issuer:    "enerzia-admin",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)

	// Wrong issuer — same secret but "enerzia-api".
	wrongIssuer, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  "ops@enerzia.in",
		"iss":  "enerzia-api",
		"role": "admin",
		"exp":  time.Now().Add(time.Hour).Unix(),
	}).SignedString(adminSecret)

	// Missing role claim.
	missingRole, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   "ops@enerzia.in",
		Issuer:    "enerzia-admin",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}).SignedString(adminSecret)

	// Wrong role.
	wrongRole, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  "ops@enerzia.in",
		"iss":  "enerzia-admin",
		"role": "superuser",
		"exp":  time.Now().Add(time.Hour).Unix(),
	}).SignedString(adminSecret)

	// No expiry.
	noExpiry, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  "ops@enerzia.in",
		"iss":  "enerzia-admin",
		"role": "admin",
	}).SignedString(adminSecret)

	tests := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"not a jwt", "hello"},
		{"truncated", valid[:len(valid)-6]},
		{"signed with another key", foreign},
		{"alg none", unsigned},
		{"wrong issuer", wrongIssuer},
		{"missing role", missingRole},
		{"wrong role", wrongRole},
		{"no expiry", noExpiry},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := issuer.Parse(tt.token); !errors.Is(err, admin.ErrInvalidToken) {
				t.Errorf("Parse(%q) error = %v, want ErrInvalidToken", tt.name, err)
			}
		})
	}
}

func TestAdminParseRejectsExpiredToken(t *testing.T) {
	issuer := admin.NewTokenIssuer(adminSecret, time.Hour)

	past := admin.NewTokenIssuerAt(adminSecret, time.Hour, func() time.Time {
		return time.Now().Add(-2 * time.Hour)
	})
	token, _, err := past.Issue("ops@enerzia.in")
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if _, err := issuer.Parse(token); !errors.Is(err, admin.ErrInvalidToken) {
		t.Errorf("Parse() error = %v, want ErrInvalidToken for an expired token", err)
	}
}

// TestCrossIssuerRejection verifies that an admin token is rejected by the
// customer auth issuer, and vice versa.
func TestCrossIssuerRejection(t *testing.T) {
	sharedSecret := []byte("shared-secret-used-for-cross-issuer-test!!")

	adminIssuer := admin.NewTokenIssuer(sharedSecret, admin.TokenTTL)
	authIssuer := auth.NewTokenIssuer(sharedSecret, auth.TokenTTL)

	adminToken, _, err := adminIssuer.Issue("ops@enerzia.in")
	if err != nil {
		t.Fatalf("admin Issue() error = %v", err)
	}

	// An admin token must NOT be accepted by the customer auth parser.
	if _, err := authIssuer.Parse(adminToken); !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("auth.Parse(admin token) error = %v, want ErrInvalidToken (cross-issuer must be blocked)", err)
	}

	// A customer token must NOT be accepted by the admin parser.
	// Build a minimal customer token — auth.Issue needs a User.
	// We use the JWT library directly to build a token with the customer issuer.
	customerToken, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   "000000000000000000000001",
		"phone": "9876543210",
		"iss":   "enerzia-api",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}).SignedString(sharedSecret)

	if _, err := adminIssuer.Parse(customerToken); !errors.Is(err, admin.ErrInvalidToken) {
		t.Errorf("admin.Parse(customer token) error = %v, want ErrInvalidToken (cross-issuer must be blocked)", err)
	}
}
