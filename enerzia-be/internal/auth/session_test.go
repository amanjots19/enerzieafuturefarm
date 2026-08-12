package auth_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/enerzia/enerzia-be/internal/auth"
	"github.com/enerzia/enerzia-be/internal/config"
	"github.com/enerzia/enerzia-be/internal/msg91"
	"github.com/enerzia/enerzia-be/internal/server"
)

// stubVerifier is an in-memory msg91.Verifier for tests.
type stubVerifier struct {
	phone string
	err   error
}

func (s *stubVerifier) VerifyAccessToken(_ context.Context, _ string) (string, error) {
	return s.phone, s.err
}

func newSessionService(t *testing.T, store auth.Store, verifier *stubVerifier) *auth.Service {
	t.Helper()
	return auth.NewService(auth.ServiceConfig{
		Store:    store,
		Sender:   auth.UnconfiguredSender{},
		Tokens:   auth.NewTokenIssuer(tokenSecret, auth.TokenTTL),
		Pepper:   pepper,
		Verifier: verifier,
	})
}

func newSessionAPI(t *testing.T, store auth.Store, verifier *stubVerifier) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := newSessionService(t, store, verifier)
	return server.New(server.Deps{
		Config:  config.Config{},
		Mongo:   stubPinger{},
		Auth:    auth.NewHandler(svc, logger),
		Logger:  logger,
		Version: "test",
		Started: time.Now(),
	})
}

/* -------------------------------------------------- CreateSession service */

func TestCreateSessionStripsCountryCode(t *testing.T) {
	// TRAP 2: MSG91 returns 12-digit "919999999999"; users.phone is 10 digits.
	verifier := &stubVerifier{phone: "919876543210"} // with country code
	svc := newSessionService(t, newMemStore(), verifier)

	result, err := svc.CreateSession(t.Context(), "access-token")
	if err != nil {
		t.Fatalf("CreateSession() error = %v, want nil", err)
	}
	if result.User.Phone != "9876543210" {
		t.Errorf("phone = %q, want 9876543210 (country code stripped)", result.User.Phone)
	}
	if result.Token == "" {
		t.Error("no token returned")
	}
	if !result.ExpiresAt.After(time.Now()) {
		t.Error("expiresAt is in the past")
	}
}

func TestCreateSessionRejectsNonNormalizablePhone(t *testing.T) {
	// A phone that does not reduce to 10 digits must be rejected, never stored.
	// Storing it would silently create a second account for the same shopper.
	cases := []struct {
		name  string
		phone string
	}{
		{name: "5 digits", phone: "91999"},
		{name: "11 digits starting with 91", phone: "91999999999"},
		{name: "13 digits", phone: "9199999999991"},
		{name: "non-digits", phone: "9Xabc123456"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newSessionService(t, newMemStore(), &stubVerifier{phone: tc.phone})
			_, err := svc.CreateSession(t.Context(), "access-token")
			if !errors.Is(err, auth.ErrSessionRejected) {
				t.Errorf("CreateSession(phone=%q) error = %v, want ErrSessionRejected", tc.phone, err)
			}
		})
	}
}

func TestCreateSessionMapsVerificationFailureToRejected(t *testing.T) {
	verifier := &stubVerifier{err: msg91.ErrVerificationFailed}
	svc := newSessionService(t, newMemStore(), verifier)

	_, err := svc.CreateSession(t.Context(), "bad-token")
	if !errors.Is(err, auth.ErrSessionRejected) {
		t.Errorf("error = %v, want ErrSessionRejected", err)
	}
}

func TestCreateSessionMapsUnconfiguredToRejected(t *testing.T) {
	// msg91.Unconfigured returns ErrNotConfigured, which is a rejection.
	verifier := &stubVerifier{err: msg91.ErrNotConfigured}
	svc := newSessionService(t, newMemStore(), verifier)

	_, err := svc.CreateSession(t.Context(), "any-token")
	if !errors.Is(err, auth.ErrSessionRejected) {
		t.Errorf("error = %v, want ErrSessionRejected", err)
	}
}

func TestCreateSessionMapsVerifierErrorToGatewayError(t *testing.T) {
	verifier := &stubVerifier{err: errors.New("connection refused")}
	svc := newSessionService(t, newMemStore(), verifier)

	_, err := svc.CreateSession(t.Context(), "access-token")
	if !errors.Is(err, auth.ErrGatewayError) {
		t.Errorf("error = %v, want ErrGatewayError", err)
	}
}

func TestCreateSessionSurfacesUpsertFailure(t *testing.T) {
	store := newMemStore()
	store.upsertErr = errors.New("mongo: write failed")
	verifier := &stubVerifier{phone: "919876543210"}
	svc := newSessionService(t, store, verifier)

	_, err := svc.CreateSession(t.Context(), "access-token")
	if err == nil {
		t.Fatal("error = nil, want the upsert failure to surface")
	}
}

func TestCreateSessionCreatesUserOnFirstSignIn(t *testing.T) {
	store := newMemStore()
	verifier := &stubVerifier{phone: "919876543210"}
	svc := newSessionService(t, store, verifier)

	r1, err := svc.CreateSession(t.Context(), "token-one")
	if err != nil {
		t.Fatalf("first CreateSession() error = %v", err)
	}
	r2, err := svc.CreateSession(t.Context(), "token-two")
	if err != nil {
		t.Fatalf("second CreateSession() error = %v", err)
	}
	if r1.User.ID != r2.User.ID {
		t.Errorf("two sign-ins produced different user IDs: %v vs %v", r1.User.ID, r2.User.ID)
	}
}

/* ----------------------------------------- POST /api/v1/auth/session handler */

func TestSessionEndpointAccepts200(t *testing.T) {
	verifier := &stubVerifier{phone: "919876543210"}
	h := newSessionAPI(t, newMemStore(), verifier)

	rec := post(t, h, "/api/v1/auth/session", `{"accessToken":"valid-token"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}

	var body struct {
		Data struct {
			Token     string    `json:"token"`
			ExpiresAt time.Time `json:"expiresAt"`
			User      struct {
				ID    string `json:"id"`
				Phone string `json:"phone"`
			} `json:"user"`
		} `json:"data"`
	}
	decodeJSONInto(t, rec, &body)

	if body.Data.Token == "" {
		t.Error("token is empty")
	}
	if body.Data.User.Phone != "9876543210" {
		t.Errorf("user.phone = %q, want 9876543210", body.Data.User.Phone)
	}
	if body.Data.User.ID == "" {
		t.Error("user.id is empty")
	}
	if !body.Data.ExpiresAt.After(time.Now()) {
		t.Error("expiresAt is in the past")
	}
}

func TestSessionEndpointRejectsEmptyAccessToken(t *testing.T) {
	h := newSessionAPI(t, newMemStore(), &stubVerifier{})

	tests := []struct {
		name string
		body string
	}{
		{name: "empty string", body: `{"accessToken":""}`},
		{name: "whitespace only", body: `{"accessToken":"   "}`},
		{name: "missing field", body: `{}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := post(t, h, "/api/v1/auth/session", tt.body)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("status = %d, want 422", rec.Code)
			}
			if code := apiErrorCode(t, rec); code != "VALIDATION_ERROR" {
				t.Errorf("error.code = %q, want VALIDATION_ERROR", code)
			}
			if msg := messageOf(t, rec); msg != "Please sign in again." {
				t.Errorf("message = %q, want \"Please sign in again.\"", msg)
			}
		})
	}
}

func TestSessionEndpointReturns401ForVerificationFailure(t *testing.T) {
	// type:"error" with HTTP 200 from MSG91 (the trap) propagates as ErrSessionRejected → 401.
	verifier := &stubVerifier{err: msg91.ErrVerificationFailed}
	h := newSessionAPI(t, newMemStore(), verifier)

	rec := post(t, h, "/api/v1/auth/session", `{"accessToken":"rejected-token"}`)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (%s)", rec.Code, rec.Body.String())
	}
	if code := apiErrorCode(t, rec); code != "UNAUTHORIZED" {
		t.Errorf("error.code = %q, want UNAUTHORIZED", code)
	}
	// One message for every rejection — nothing about what check failed.
	if msg := messageOf(t, rec); msg != "We could not verify that sign-in. Please try again." {
		t.Errorf("message = %q, unexpected", msg)
	}
}

func TestSessionEndpointReturns401ForUnconfiguredVerifier(t *testing.T) {
	verifier := &stubVerifier{err: msg91.ErrNotConfigured}
	h := newSessionAPI(t, newMemStore(), verifier)

	rec := post(t, h, "/api/v1/auth/session", `{"accessToken":"any"}`)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestSessionEndpointReturns502ForGatewayError(t *testing.T) {
	verifier := &stubVerifier{err: errors.New("connection refused")}
	h := newSessionAPI(t, newMemStore(), verifier)

	rec := post(t, h, "/api/v1/auth/session", `{"accessToken":"any"}`)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 (%s)", rec.Code, rec.Body.String())
	}
	if code := apiErrorCode(t, rec); code != "GATEWAY_ERROR" {
		t.Errorf("error.code = %q, want GATEWAY_ERROR", code)
	}
}

func TestSessionEndpointRejectsMalformedBody(t *testing.T) {
	h := newSessionAPI(t, newMemStore(), &stubVerifier{})

	tests := []struct {
		name string
		body string
	}{
		{name: "not json", body: `nope`},
		{name: "truncated", body: `{"accessToken":`},
		{name: "unknown field", body: `{"token":"abc"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := post(t, h, "/api/v1/auth/session", tt.body)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rec.Code)
			}
		})
	}
}

func TestSessionEndpointClientMessageIsGenericWithTypedError(t *testing.T) {
	// When MSG91 returns a typed error carrying a diagnostic code, the 401
	// response must still show the generic message — never MSG91's reason.
	verifier := &stubVerifier{
		err: &msg91.VerificationError{
			Type:    "error",
			Code:    "418",
			Message: "AuthenticationFailure",
		},
	}
	h := newSessionAPI(t, newMemStore(), verifier)

	rec := post(t, h, "/api/v1/auth/session", `{"accessToken":"test-token"}`)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if code := apiErrorCode(t, rec); code != "UNAUTHORIZED" {
		t.Errorf("error.code = %q, want UNAUTHORIZED", code)
	}
	const wantMsg = "We could not verify that sign-in. Please try again."
	if msg := messageOf(t, rec); msg != wantMsg {
		t.Errorf("message = %q, want generic %q — MSG91's reason must not reach the client", msg, wantMsg)
	}
	// Confirm MSG91's diagnostic code and reason did not leak into the body.
	body := rec.Body.String()
	if containsAny(body, "418", "AuthenticationFailure") {
		t.Errorf("response body contains MSG91 diagnostic detail: %s", body)
	}
}

func TestSessionEndpointGivesOneMessageForEveryRejection(t *testing.T) {
	// All rejection paths must return the same message so nothing is enumerable.
	msgs := map[string]bool{}

	rejected := &stubVerifier{err: msg91.ErrVerificationFailed}
	h := newSessionAPI(t, newMemStore(), rejected)
	msgs[messageOf(t, post(t, h, "/api/v1/auth/session", `{"accessToken":"any"}`))] = true

	unconfigured := &stubVerifier{err: msg91.ErrNotConfigured}
	h2 := newSessionAPI(t, newMemStore(), unconfigured)
	msgs[messageOf(t, post(t, h2, "/api/v1/auth/session", `{"accessToken":"any"}`))] = true

	badPhone := &stubVerifier{phone: "12345"} // 5 digits, not normalisable
	h3 := newSessionAPI(t, newMemStore(), badPhone)
	msgs[messageOf(t, post(t, h3, "/api/v1/auth/session", `{"accessToken":"any"}`))] = true

	if len(msgs) != 1 {
		t.Errorf("rejection messages differ by cause: %v", msgs)
	}
}

func TestSessionEndpointHidesInternalFailures(t *testing.T) {
	store := newMemStore()
	store.upsertErr = errors.New("auth failed for user appuser")
	verifier := &stubVerifier{phone: "919876543210"}
	h := newSessionAPI(t, store, verifier)

	rec := post(t, h, "/api/v1/auth/session", `{"accessToken":"valid-token"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if body := rec.Body.String(); containsAny(body, "appuser", "mongodb") {
		t.Errorf("response leaks internals: %s", body)
	}
}

// containsAny reports whether s contains any of the given substrings.
func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if len(sub) > 0 {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
