package msg91_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/enerzia/enerzia-be/internal/msg91"
)

// replyJSON writes a JSON body with the given status code.
func replyJSON(code int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
		if body != "" {
			_, _ = w.Write([]byte(body))
		}
	}
}

func TestVerifyAccessToken(t *testing.T) {
	tests := []struct {
		name        string
		handler     http.HandlerFunc
		cancel      bool
		wantPhone   string
		wantErrIs   error // nil means "just assert err != nil"
		wantNoError bool
	}{
		{
			// TRAP: MSG91 may return HTTP 200 with type "error".
			// Checking the HTTP status alone would accept a failed verification.
			name:      "type error with HTTP 200 is a rejection",
			handler:   replyJSON(http.StatusOK, `{"type":"error","message":"invalid token"}`),
			wantErrIs: msg91.ErrVerificationFailed,
		},
		{
			name:    "non-2xx from MSG91",
			handler: replyJSON(http.StatusUnauthorized, ``),
			// Not ErrVerificationFailed — it is a transport/gateway failure.
		},
		{
			name:    "malformed JSON response",
			handler: replyJSON(http.StatusOK, `{not valid json`),
		},
		{
			name:   "cancelled context honours cancellation",
			cancel: true,
		},
		{
			name:        "success returns raw phone",
			handler:     replyJSON(http.StatusOK, `{"type":"success","message":"919999999999"}`),
			wantPhone:   "919999999999",
			wantNoError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := tt.handler
			if tt.cancel {
				// Hang indefinitely; the pre-cancelled context stops the call.
				handler = func(w http.ResponseWriter, _ *http.Request) {
					time.Sleep(10 * time.Second)
				}
			}

			srv := httptest.NewServer(handler)
			defer srv.Close()

			client := msg91.NewClientWithBaseURL("test-authkey", srv.URL)

			ctx := t.Context()
			if tt.cancel {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			phone, err := client.VerifyAccessToken(ctx, "some-access-token")

			if tt.wantNoError {
				if err != nil {
					t.Fatalf("VerifyAccessToken() error = %v, want nil", err)
				}
				if phone != tt.wantPhone {
					t.Errorf("phone = %q, want %q", phone, tt.wantPhone)
				}
				return
			}

			if err == nil {
				t.Fatal("VerifyAccessToken() error = nil, want non-nil")
			}
			if phone != "" {
				t.Errorf("phone = %q on error, want empty", phone)
			}
			if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
				t.Errorf("errors.Is(%T) = false, want true; got %v", tt.wantErrIs, err)
			}
		})
	}
}

func TestVerifyAccessTokenSendsAuthKeyAndBody(t *testing.T) {
	var gotKey, gotContentType string
	var gotBody map[string]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("authkey")
		gotContentType = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"type":"success","message":"919999999999"}`))
	}))
	defer srv.Close()

	client := msg91.NewClientWithBaseURL("my-auth-key", srv.URL)
	_, _ = client.VerifyAccessToken(t.Context(), "my-access-token")

	if gotKey != "my-auth-key" {
		t.Errorf("authkey header = %q, want %q", gotKey, "my-auth-key")
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}
	if gotBody["access-token"] != "my-access-token" {
		t.Errorf("body access-token = %q, want my-access-token", gotBody["access-token"])
	}
}

func TestVerifyAccessTokenTypedErrorCarriesCode(t *testing.T) {
	// MSG91's real rejection response includes code and message. Verify that
	// VerifyAccessToken returns a *VerificationError with those fields, AND
	// that errors.Is(err, ErrVerificationFailed) still holds so callers using
	// the sentinel check are unaffected.
	srv := httptest.NewServer(replyJSON(http.StatusOK,
		`{"type":"error","code":"418","message":"AuthenticationFailure"}`))
	defer srv.Close()

	client := msg91.NewClientWithBaseURL("test-key", srv.URL)
	_, err := client.VerifyAccessToken(t.Context(), "bad-token")

	if err == nil {
		t.Fatal("VerifyAccessToken() error = nil, want *VerificationError")
	}

	var ve *msg91.VerificationError
	if !errors.As(err, &ve) {
		t.Fatalf("errors.As(*VerificationError) = false; got %T: %v", err, err)
	}
	if ve.Code != "418" {
		t.Errorf("Code = %q, want 418", ve.Code)
	}
	if ve.Message != "AuthenticationFailure" {
		t.Errorf("Message = %q, want AuthenticationFailure", ve.Message)
	}
	if ve.Type != "error" {
		t.Errorf("Type = %q, want error", ve.Type)
	}
	// Sentinel check must still work for callers that only test for rejection.
	if !errors.Is(err, msg91.ErrVerificationFailed) {
		t.Error("errors.Is(ErrVerificationFailed) = false, want true")
	}
	// Error() must mention the code so log lines are self-explanatory.
	if got := err.Error(); !strings.Contains(got, "418") {
		t.Errorf("Error() = %q, want it to contain the MSG91 code", got)
	}
}

func TestVerifyAccessTokenTypedErrorCodeAsNumber(t *testing.T) {
	// MSG91 returns code as a JSON number in some responses (e.g. 701 for
	// "invalid access-token"). Verify the decoder handles it without error.
	srv := httptest.NewServer(replyJSON(http.StatusOK,
		`{"type":"error","code":701,"message":"invalid access-token"}`))
	defer srv.Close()

	client := msg91.NewClientWithBaseURL("test-key", srv.URL)
	_, err := client.VerifyAccessToken(t.Context(), "bad-token")

	var ve *msg91.VerificationError
	if !errors.As(err, &ve) {
		t.Fatalf("errors.As(*VerificationError) = false; got %T: %v", err, err)
	}
	if ve.Code != "701" {
		t.Errorf("Code = %q, want 701", ve.Code)
	}
	if !errors.Is(err, msg91.ErrVerificationFailed) {
		t.Error("errors.Is(ErrVerificationFailed) = false")
	}
}

func TestUnconfiguredReturnsErrNotConfigured(t *testing.T) {
	_, err := msg91.Unconfigured{}.VerifyAccessToken(t.Context(), "any-token")
	if !errors.Is(err, msg91.ErrNotConfigured) {
		t.Errorf("VerifyAccessToken() error = %v, want ErrNotConfigured", err)
	}
}

func TestNewClientDefaultsToProductionURL(t *testing.T) {
	c := msg91.NewClient("key")
	if msg91.ClientBaseURL(c) != "https://api.msg91.com" {
		t.Errorf("base URL = %q, want https://api.msg91.com", msg91.ClientBaseURL(c))
	}
	if msg91.ClientTimeout(c) != 10*time.Second {
		t.Errorf("timeout = %v, want 10s", msg91.ClientTimeout(c))
	}
}

func TestNewClientUsesIPv4Dialer(t *testing.T) {
	// MSG91 enforces an IP allowlist and only the IPv4 egress is registered.
	// If this test fails, every real verification call will get code 418
	// (AuthenticationFailure) because it egresses from an unwhitelisted IPv6
	// address — a failure that is invisible until production traffic runs.
	c := msg91.NewClient("key")
	if !msg91.ClientHasIPv4Dialer(c) {
		t.Error("NewClient must configure an IPv4-only DialContext on its transport; " +
			"removing it causes MSG91 to reject all calls with 418 when IPv6 is preferred")
	}
}
