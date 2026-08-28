package auth_test

import (
	"strings"
	"testing"
	"time"

	"github.com/enerzia/enerzia-be/internal/auth"
)

var pepper = []byte("a-test-pepper-at-least-32-characters")

func TestValidPhone(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		// Stored form: digits, country code included, no '+'.
		{"919876543210", true},    // India
		{"12025551234", true},     // US
		{"447700900123", true},    // UK
		{"4712345678", true},      // Norway — exactly ten digits, and foreign
		{"12345678", true},        // the 8-digit floor
		{"123456789012345", true}, // E.164's 15-digit ceiling
		// A legacy ten-digit document predates the country code being stored and
		// stays valid until it is migrated (schema.md §users).
		{"9876543210", true},
		{"0000000000", true},
		{"1234567", false},          // below the floor
		{"1234567890123456", false}, // past E.164's ceiling
		{"+919876543210", false},    // the '+' is trimmed by normalizePhone, not here
		{"98765 43210", false},
		{"98765abcde", false},
		{"ananya@example.com", false}, // a non-mobile MSG91 widget identifier
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := auth.ValidPhone(tt.in); got != tt.want {
				t.Errorf("ValidPhone(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestValidCodeShape(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"123456", true},
		{"000000", true},
		{"12345", false},
		{"1234567", false},
		{"12345a", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := auth.ValidCodeShape(tt.in); got != tt.want {
				t.Errorf("ValidCodeShape(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestGenerateCodeIsSixDigitsAndVaries(t *testing.T) {
	seen := make(map[string]bool)
	for range 200 {
		code, err := auth.GenerateCode()
		if err != nil {
			t.Fatalf("GenerateCode() error = %v", err)
		}
		if len(code) != auth.CodeLength {
			t.Fatalf("GenerateCode() = %q, want %d characters", code, auth.CodeLength)
		}
		if !auth.ValidCodeShape(code) {
			t.Fatalf("GenerateCode() = %q, which is not six digits", code)
		}
		seen[code] = true
	}
	// 200 draws from a million values colliding into a handful would mean the
	// generator is not random.
	if len(seen) < 150 {
		t.Errorf("only %d distinct codes in 200 draws; generator looks weak", len(seen))
	}
}

func TestGenerateCodeKeepsLeadingZeros(t *testing.T) {
	// Every code must be six characters, or a short one leaks its own range.
	for range 5000 {
		code, err := auth.GenerateCode()
		if err != nil {
			t.Fatalf("GenerateCode() error = %v", err)
		}
		if len(code) != 6 {
			t.Fatalf("GenerateCode() = %q, want six characters including leading zeros", code)
		}
	}
}

func TestHashCodeIsStableAndKeyed(t *testing.T) {
	const phone, code = "9876543210", "123456"

	first := auth.HashCode(pepper, phone, code)
	if first != auth.HashCode(pepper, phone, code) {
		t.Error("HashCode() is not deterministic")
	}
	if first == "" {
		t.Fatal("HashCode() returned an empty string")
	}
	// The stored value must be useless without the secret.
	if other := auth.HashCode([]byte("a-different-pepper-entirely-here"), phone, code); other == first {
		t.Error("HashCode() ignores the pepper; a database leak would be enough to sign in")
	}
	if strings.Contains(first, code) {
		t.Errorf("HashCode() = %q, which contains the plaintext code", first)
	}
}

func TestHashCodeIsBoundToThePhoneNumber(t *testing.T) {
	// Otherwise a hash observed for one number could be replayed on another.
	const code = "123456"
	if auth.HashCode(pepper, "9876543210", code) == auth.HashCode(pepper, "9000000000", code) {
		t.Error("HashCode() is identical across phone numbers")
	}
}

func TestCodeMatches(t *testing.T) {
	const phone, code = "9876543210", "123456"
	stored := auth.HashCode(pepper, phone, code)

	tests := []struct {
		name   string
		pepper []byte
		phone  string
		code   string
		want   bool
	}{
		{name: "correct", pepper: pepper, phone: phone, code: code, want: true},
		{name: "wrong code", pepper: pepper, phone: phone, code: "654321"},
		{name: "wrong phone", pepper: pepper, phone: "9000000000", code: code},
		{name: "wrong pepper", pepper: []byte("nope-nope-nope-nope-nope-nope-32"), phone: phone, code: code},
		{name: "empty code", pepper: pepper, phone: phone, code: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := auth.CodeMatches(tt.pepper, tt.phone, tt.code, stored)
			if got != tt.want {
				t.Errorf("CodeMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCodeMatchesRejectsAGarbageStoredHash(t *testing.T) {
	if auth.CodeMatches(pepper, "9876543210", "123456", "") {
		t.Error("CodeMatches() accepted an empty stored hash")
	}
	if auth.CodeMatches(pepper, "9876543210", "123456", "not-hex") {
		t.Error("CodeMatches() accepted a malformed stored hash")
	}
}

func TestOTPCodeUsable(t *testing.T) {
	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	fresh := auth.OTPCode{ExpiresAt: now.Add(time.Minute)}

	tests := []struct {
		name string
		code auth.OTPCode
		want bool
	}{
		{name: "fresh", code: fresh, want: true},
		{
			name: "already used",
			code: auth.OTPCode{Consumed: true, ExpiresAt: now.Add(time.Minute)},
		},
		{
			name: "expired",
			code: auth.OTPCode{ExpiresAt: now.Add(-time.Second)},
		},
		{
			name: "expiring exactly now",
			code: auth.OTPCode{ExpiresAt: now},
		},
		{
			name: "locked out",
			code: auth.OTPCode{Attempts: auth.MaxAttempts, ExpiresAt: now.Add(time.Minute)},
		},
		{
			name: "one attempt left",
			code: auth.OTPCode{Attempts: auth.MaxAttempts - 1, ExpiresAt: now.Add(time.Minute)},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.code.Usable(now); got != tt.want {
				t.Errorf("Usable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPolicyMatchesTheRoadmap(t *testing.T) {
	// These numbers are the contract; changing one silently changes security.
	if auth.CodeTTL != 5*time.Minute {
		t.Errorf("CodeTTL = %v, want 5m", auth.CodeTTL)
	}
	if auth.MaxAttempts != 5 {
		t.Errorf("MaxAttempts = %d, want 5", auth.MaxAttempts)
	}
	if auth.TokenTTL != 30*24*time.Hour {
		t.Errorf("TokenTTL = %v, want 30 days", auth.TokenTTL)
	}
	if auth.ResendInterval != 30*time.Second {
		t.Errorf("ResendInterval = %v, want 30s", auth.ResendInterval)
	}
	if auth.RateWindow != 10*time.Minute || auth.MaxRequestsPerWindow != 3 {
		t.Errorf("rate limit = %d per %v, want 3 per 10m", auth.MaxRequestsPerWindow, auth.RateWindow)
	}
}
