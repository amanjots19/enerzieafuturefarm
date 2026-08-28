package auth_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
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
	h, _ := newSessionAPIWithLog(t, store, verifier)
	return h
}

// newSessionAPIWithLog is newSessionAPI with the log captured, so a test can
// assert what an operator would actually see in the journal.
func newSessionAPIWithLog(t *testing.T, store auth.Store, verifier *stubVerifier) (http.Handler, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	svc := newSessionService(t, store, verifier)
	return server.New(server.Deps{
		Config:  config.Config{},
		Mongo:   stubPinger{},
		Auth:    auth.NewHandler(svc, logger),
		Logger:  logger,
		Version: "test",
		Started: time.Now(),
	}), &buf
}

/* -------------------------------------------------- CreateSession service */

func TestCreateSessionKeepsTheCountryCode(t *testing.T) {
	// The country code is STORED, not stripped. Stripping it is what rejected
	// every non-Indian number after MSG91 had verified it — see roadmap.md
	// §Auth. This test exists to stop that being reintroduced.
	verifier := &stubVerifier{phone: "919876543210"}
	svc := newSessionService(t, newMemStore(), verifier)

	result, err := svc.CreateSession(t.Context(), "access-token")
	if err != nil {
		t.Fatalf("CreateSession() error = %v, want nil", err)
	}
	if result.User.Phone != "919876543210" {
		t.Errorf("phone = %q, want 919876543210 — the country code must be kept", result.User.Phone)
	}
	if result.Token == "" {
		t.Error("no token returned")
	}
	if !result.ExpiresAt.After(time.Now()) {
		t.Error("expiresAt is in the past")
	}
}

// TestCreateSessionAcceptsInternationalNumbers is the regression test for the
// bug this whole change exists to fix: MSG91 verified these, and the server
// refused them anyway.
func TestCreateSessionAcceptsInternationalNumbers(t *testing.T) {
	cases := []struct {
		name  string
		phone string
	}{
		{name: "US", phone: "12025551234"},
		{name: "UK", phone: "447700900123"},
		{name: "UAE", phone: "971501234567"},
		{name: "Norway, ten digits and not Indian", phone: "4712345678"},
		{name: "already carries a plus", phone: "+12025551234"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newSessionService(t, newMemStore(), &stubVerifier{phone: tc.phone})
			result, err := svc.CreateSession(t.Context(), "access-token")
			if err != nil {
				t.Fatalf("CreateSession(phone=%q) error = %v, want nil", tc.phone, err)
			}
			want := strings.TrimPrefix(tc.phone, "+")
			if result.User.Phone != want {
				t.Errorf("phone = %q, want %q", result.User.Phone, want)
			}
		})
	}
}

func TestCreateSessionRejectsNonNormalizablePhone(t *testing.T) {
	// Rejected, never stored: a malformed identity would silently create a
	// second account for the same shopper.
	cases := []struct {
		name  string
		phone string
	}{
		{name: "5 digits", phone: "91999"},
		{name: "past E.164's ceiling", phone: "9199999999999999"},
		{name: "non-digits", phone: "9Xabc123456"},
		{name: "an email, from a widget that is not mobile-only", phone: "ananya@example.com"},
		{name: "empty", phone: ""},
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

// TestCreateSessionReportsTheShapeOfARejectedIdentifier pins the diagnostic
// that was missing when this bug was reported live: a rejection here and a
// rejection by MSG91 wrote the same log line, so neither could be ruled out.
func TestCreateSessionReportsTheShapeOfARejectedIdentifier(t *testing.T) {
	svc := newSessionService(t, newMemStore(), &stubVerifier{phone: "ananya@example.com"})

	_, err := svc.CreateSession(t.Context(), "access-token")

	var pe *auth.PhoneRejectedError
	if !errors.As(err, &pe) {
		t.Fatalf("error = %v, want a *PhoneRejectedError the handler can log", err)
	}
	if pe.Digits != len("ananya@example.com") {
		t.Errorf("Digits = %d, want %d", pe.Digits, len("ananya@example.com"))
	}
	if pe.Prefix != "an" {
		t.Errorf("Prefix = %q, want %q", pe.Prefix, "an")
	}
	// The number itself must never travel in the error — it ends up in a log.
	if strings.Contains(pe.Error(), "ananya@example.com") {
		t.Error("the rejected identifier leaked into the error message")
	}
	// It is still a rejection, so the client still gets one 401.
	if !errors.Is(err, auth.ErrSessionRejected) {
		t.Error("a shape-carrying rejection must still be an ErrSessionRejected")
	}
}

func TestPhoneRejectedErrorPrefixHandlesShortValues(t *testing.T) {
	svc := newSessionService(t, newMemStore(), &stubVerifier{phone: "7"})

	_, err := svc.CreateSession(t.Context(), "access-token")

	var pe *auth.PhoneRejectedError
	if !errors.As(err, &pe) {
		t.Fatalf("error = %v, want a *PhoneRejectedError", err)
	}
	if pe.Prefix != "7" {
		t.Errorf("Prefix = %q, want %q — a value shorter than the prefix must not panic", pe.Prefix, "7")
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
	// The country code reaches the client, per roadmap.md §Auth's 200 example.
	if body.Data.User.Phone != "919876543210" {
		t.Errorf("user.phone = %q, want 919876543210", body.Data.User.Phone)
	}
	if body.Data.User.ID == "" {
		t.Error("user.id is empty")
	}
	if !body.Data.ExpiresAt.After(time.Now()) {
		t.Error("expiresAt is in the past")
	}
}

// TestSessionEndpointLogsWhyItRejected is the test this whole change owes its
// existence to. When international sign-in broke, "MSG91 said no" and "we said
// no" wrote the same log line, so the cause could not be read off the journal.
// These two must stay distinguishable.
func TestSessionEndpointLogsWhyItRejected(t *testing.T) {
	t.Run("our own validation refused the identifier", func(t *testing.T) {
		h, log := newSessionAPIWithLog(t, newMemStore(), &stubVerifier{phone: "ananya@example.com"})

		rec := post(t, h, "/api/v1/auth/session", `{"accessToken":"valid-token"}`)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}

		out := log.String()
		for _, want := range []string{`"reason":"phone_unusable"`, `"phone_prefix":"an"`, `"phone_chars":18`} {
			if !strings.Contains(out, want) {
				t.Errorf("log is missing %s\nlog: %s", want, out)
			}
		}
		// The identifier itself must never be written to the journal.
		if strings.Contains(out, "ananya@example.com") {
			t.Error("the rejected identifier was logged — that is a customer's contact detail")
		}
		if strings.Contains(out, "msg91_code") {
			t.Error("a rejection by us must not be logged as one by MSG91")
		}
	})

	t.Run("MSG91 refused the token", func(t *testing.T) {
		verifier := &stubVerifier{err: &msg91.VerificationError{
			Type: "error", Code: "invalid", Message: "token expired",
		}}
		h, log := newSessionAPIWithLog(t, newMemStore(), verifier)

		rec := post(t, h, "/api/v1/auth/session", `{"accessToken":"stale-token"}`)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}

		out := log.String()
		if !strings.Contains(out, `"reason":"msg91_rejected"`) {
			t.Errorf("log is missing the MSG91 reason\nlog: %s", out)
		}
		if strings.Contains(out, "phone_prefix") {
			t.Error("an MSG91 rejection must not be logged as one of ours")
		}
	})

	// Both answer the client identically: which check failed is not the
	// shopper's business, and enumerable if it were.
	t.Run("the client cannot tell them apart", func(t *testing.T) {
		ours := post(t, newSessionAPI(t, newMemStore(), &stubVerifier{phone: "nope@example.com"}),
			"/api/v1/auth/session", `{"accessToken":"valid-token"}`)
		theirs := post(t, newSessionAPI(t, newMemStore(), &stubVerifier{err: msg91.ErrVerificationFailed}),
			"/api/v1/auth/session", `{"accessToken":"bad-token"}`)

		if ours.Code != theirs.Code || ours.Body.String() != theirs.Body.String() {
			t.Errorf("responses differ:\n ours   %d %s\n theirs %d %s",
				ours.Code, ours.Body.String(), theirs.Code, theirs.Body.String())
		}
	})
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
