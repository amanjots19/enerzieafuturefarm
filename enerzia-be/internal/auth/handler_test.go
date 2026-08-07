package auth_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/enerzia/enerzia-be/internal/auth"
	"github.com/enerzia/enerzia-be/internal/config"
	"github.com/enerzia/enerzia-be/internal/server"
)

type stubPinger struct{}

func (stubPinger) Ping(context.Context) error { return nil }

// newAuthAPI wires a store through the real service, handler and router, so
// these exercise routing, middleware and encoding together.
func newAuthAPI(t *testing.T, store auth.Store, sender auth.Sender, reveal bool) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := auth.NewService(auth.ServiceConfig{
		Store:      store,
		Sender:     sender,
		Tokens:     auth.NewTokenIssuer(tokenSecret, auth.TokenTTL),
		Pepper:     pepper,
		RevealCode: reveal,
	})
	return server.New(server.Deps{
		Config:  config.Config{},
		Mongo:   stubPinger{},
		Auth:    auth.NewHandler(svc, logger),
		Logger:  logger,
		Version: "test",
		Started: time.Now(),
	})
}

func post(t *testing.T, h http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func getWithToken(t *testing.T, h http.Handler, path, authHeader string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeJSONInto(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("body is not JSON: %v (%s)", err, rec.Body.String())
	}
}

func apiErrorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeJSONInto(t, rec, &body)
	return body.Error.Code
}

/* ------------------------------------ POST /api/v1/auth/otp/request */

func TestRequestEndpointAccepts202(t *testing.T) {
	sender := &recordingSender{}
	h := newAuthAPI(t, newMemStore(), sender, true)

	rec := post(t, h, "/api/v1/auth/otp/request", `{"phone":"9876543210"}`)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 (%s)", rec.Code, rec.Body.String())
	}

	var body struct {
		Data struct {
			ExpiresInSeconds   int    `json:"expiresInSeconds"`
			ResendAfterSeconds int    `json:"resendAfterSeconds"`
			DevCode            string `json:"devCode"`
		} `json:"data"`
	}
	decodeJSONInto(t, rec, &body)

	if body.Data.ExpiresInSeconds != 300 {
		t.Errorf("expiresInSeconds = %d, want 300", body.Data.ExpiresInSeconds)
	}
	if body.Data.ResendAfterSeconds != 30 {
		t.Errorf("resendAfterSeconds = %d, want 30", body.Data.ResendAfterSeconds)
	}
	if body.Data.DevCode != sender.code {
		t.Errorf("devCode = %q, want the sent code", body.Data.DevCode)
	}
}

func TestRequestEndpointOmitsDevCodeInProduction(t *testing.T) {
	h := newAuthAPI(t, newMemStore(), &recordingSender{}, false)

	rec := post(t, h, "/api/v1/auth/otp/request", `{"phone":"9876543210"}`)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "devCode") {
		t.Errorf("production response carries devCode: %s", rec.Body.String())
	}
}

func TestRequestEndpointRejectsABadPhone(t *testing.T) {
	h := newAuthAPI(t, newMemStore(), &recordingSender{}, true)

	rec := post(t, h, "/api/v1/auth/otp/request", `{"phone":"12345"}`)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}

	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	decodeJSONInto(t, rec, &body)

	if body.Error.Code != "VALIDATION_ERROR" {
		t.Errorf("error.code = %q, want VALIDATION_ERROR", body.Error.Code)
	}
	// The exact copy the shipped UI shows.
	if body.Error.Message != "Enter a valid 10-digit mobile number" {
		t.Errorf("message = %q, want the frontend's wording", body.Error.Message)
	}
}

func TestRequestEndpointRateLimits(t *testing.T) {
	h := newAuthAPI(t, newMemStore(), &recordingSender{}, true)

	if rec := post(t, h, "/api/v1/auth/otp/request", `{"phone":"9876543210"}`); rec.Code != http.StatusAccepted {
		t.Fatalf("first request status = %d, want 202", rec.Code)
	}
	rec := post(t, h, "/api/v1/auth/otp/request", `{"phone":"9876543210"}`)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if got := apiErrorCode(t, rec); got != "RATE_LIMITED" {
		t.Errorf("error.code = %q, want RATE_LIMITED", got)
	}
}

func TestRequestEndpointRejectsMalformedBodies(t *testing.T) {
	h := newAuthAPI(t, newMemStore(), &recordingSender{}, true)

	tests := []struct {
		name string
		body string
	}{
		{name: "not json", body: `nope`},
		{name: "truncated", body: `{"phone":`},
		{name: "empty", body: ``},
		// A typo'd field must not be silently ignored.
		{name: "unknown field", body: `{"phoneNumber":"9876543210"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := post(t, h, "/api/v1/auth/otp/request", tt.body)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rec.Code)
			}
		})
	}
}

func TestRequestEndpointHidesInternalFailures(t *testing.T) {
	store := newMemStore()
	store.createErr = errors.New("cluster0-shard-00.mongodb.net timed out")
	h := newAuthAPI(t, store, &recordingSender{}, true)

	rec := post(t, h, "/api/v1/auth/otp/request", `{"phone":"9876543210"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "mongodb.net") {
		t.Errorf("response leaks infrastructure detail: %s", rec.Body.String())
	}
}

func TestRequestEndpointNeverEchoesTheCodeOnFailure(t *testing.T) {
	store := newMemStore()
	store.createErr = errors.New("boom")
	sender := &recordingSender{}
	h := newAuthAPI(t, store, sender, true)

	rec := post(t, h, "/api/v1/auth/otp/request", `{"phone":"9876543210"}`)

	if sender.code != "" && strings.Contains(rec.Body.String(), sender.code) {
		t.Errorf("error response leaked the code: %s", rec.Body.String())
	}
}

/* ------------------------------------- POST /api/v1/auth/otp/verify */

// signIn drives a full request+verify and returns the bearer token.
func signIn(t *testing.T, h http.Handler, phone string) string {
	t.Helper()

	rec := post(t, h, "/api/v1/auth/otp/request", `{"phone":"`+phone+`"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("request status = %d (%s)", rec.Code, rec.Body.String())
	}
	var reqBody struct {
		Data struct {
			DevCode string `json:"devCode"`
		} `json:"data"`
	}
	decodeJSONInto(t, rec, &reqBody)

	rec = post(t, h, "/api/v1/auth/otp/verify",
		`{"phone":"`+phone+`","code":"`+reqBody.Data.DevCode+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("verify status = %d (%s)", rec.Code, rec.Body.String())
	}
	var verifyBody struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	decodeJSONInto(t, rec, &verifyBody)
	return verifyBody.Data.Token
}

func TestVerifyEndpointReturnsATokenAndUser(t *testing.T) {
	h := newAuthAPI(t, newMemStore(), &recordingSender{}, true)

	rec := post(t, h, "/api/v1/auth/otp/request", `{"phone":"9876543210"}`)
	var reqBody struct {
		Data struct {
			DevCode string `json:"devCode"`
		} `json:"data"`
	}
	decodeJSONInto(t, rec, &reqBody)

	rec = post(t, h, "/api/v1/auth/otp/verify",
		`{"phone":"9876543210","code":"`+reqBody.Data.DevCode+`"}`)

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
		t.Error("no token in the response")
	}
	if body.Data.User.Phone != "9876543210" {
		t.Errorf("user.phone = %q", body.Data.User.Phone)
	}
	if body.Data.User.ID == "" {
		t.Error("user.id is empty")
	}
	if !body.Data.ExpiresAt.After(time.Now()) {
		t.Error("expiresAt is in the past")
	}
}

func TestVerifyEndpointRejectsAWrongCodeWith401(t *testing.T) {
	h := newAuthAPI(t, newMemStore(), &recordingSender{}, true)
	post(t, h, "/api/v1/auth/otp/request", `{"phone":"9876543210"}`)

	rec := post(t, h, "/api/v1/auth/otp/verify", `{"phone":"9876543210","code":"000000"}`)

	// 401, not 422: the shape was fine, the credential was not.
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if got := apiErrorCode(t, rec); got != "UNAUTHORIZED" {
		t.Errorf("error.code = %q, want UNAUTHORIZED", got)
	}
}

func TestVerifyEndpointGivesOneMessageForEveryRejection(t *testing.T) {
	// Distinguishing wrong from expired from spent would help enumeration.
	h := newAuthAPI(t, newMemStore(), &recordingSender{}, true)

	messages := map[string]bool{}

	// No code requested at all.
	rec := post(t, h, "/api/v1/auth/otp/verify", `{"phone":"9000000000","code":"123456"}`)
	messages[messageOf(t, rec)] = true

	// Requested, then guessed wrong.
	post(t, h, "/api/v1/auth/otp/request", `{"phone":"9876543210"}`)
	rec = post(t, h, "/api/v1/auth/otp/verify", `{"phone":"9876543210","code":"000000"}`)
	messages[messageOf(t, rec)] = true

	if len(messages) != 1 {
		t.Errorf("rejection messages differ by cause: %v", messages)
	}
}

func messageOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	decodeJSONInto(t, rec, &body)
	return body.Error.Message
}

func TestVerifyEndpointRejectsABadCodeShapeWith422(t *testing.T) {
	h := newAuthAPI(t, newMemStore(), &recordingSender{}, true)

	rec := post(t, h, "/api/v1/auth/otp/verify", `{"phone":"9876543210","code":"12"}`)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if got := apiErrorCode(t, rec); got != "VALIDATION_ERROR" {
		t.Errorf("error.code = %q, want VALIDATION_ERROR", got)
	}
}

func TestVerifyEndpointRejectsABadPhone(t *testing.T) {
	h := newAuthAPI(t, newMemStore(), &recordingSender{}, true)

	rec := post(t, h, "/api/v1/auth/otp/verify", `{"phone":"12345","code":"123456"}`)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", rec.Code)
	}
}

func TestVerifyEndpointRejectsAMalformedBody(t *testing.T) {
	h := newAuthAPI(t, newMemStore(), &recordingSender{}, true)

	if rec := post(t, h, "/api/v1/auth/otp/verify", `{`); rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestVerifyEndpointHidesInternalFailures(t *testing.T) {
	store := newMemStore()
	h := newAuthAPI(t, store, &recordingSender{}, true)
	post(t, h, "/api/v1/auth/otp/request", `{"phone":"9876543210"}`)

	store.latestErr = errors.New("auth failed for user appuser")
	rec := post(t, h, "/api/v1/auth/otp/verify", `{"phone":"9876543210","code":"123456"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "appuser") {
		t.Errorf("response leaks driver detail: %s", rec.Body.String())
	}
}

func TestVerifyEndpointNeverEchoesTheToken(t *testing.T) {
	// A token in an error body would be a credential in a log or a cache.
	h := newAuthAPI(t, newMemStore(), &recordingSender{}, true)
	post(t, h, "/api/v1/auth/otp/request", `{"phone":"9876543210"}`)

	rec := post(t, h, "/api/v1/auth/otp/verify", `{"phone":"9876543210","code":"000000"}`)
	if strings.Contains(rec.Body.String(), "token") {
		t.Errorf("rejection body mentions a token: %s", rec.Body.String())
	}
}

/* ---------------------------------------------- GET /api/v1/auth/me */

func TestMeReturnsTheSignedInUser(t *testing.T) {
	h := newAuthAPI(t, newMemStore(), &recordingSender{}, true)
	token := signIn(t, h, "9876543210")

	rec := getWithToken(t, h, "/api/v1/auth/me", "Bearer "+token)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}

	var body struct {
		Data struct {
			User struct {
				Phone string `json:"phone"`
				ID    string `json:"id"`
			} `json:"user"`
		} `json:"data"`
	}
	decodeJSONInto(t, rec, &body)

	if body.Data.User.Phone != "9876543210" {
		t.Errorf("phone = %q", body.Data.User.Phone)
	}
}

func TestMeRejectsBadCredentials(t *testing.T) {
	h := newAuthAPI(t, newMemStore(), &recordingSender{}, true)
	valid := signIn(t, h, "9876543210")

	tests := []struct {
		name   string
		header string
	}{
		{name: "no header", header: ""},
		{name: "no scheme", header: valid},
		{name: "wrong scheme", header: "Basic " + valid},
		{name: "empty token", header: "Bearer "},
		{name: "whitespace token", header: "Bearer    "},
		{name: "garbage token", header: "Bearer not-a-jwt"},
		{name: "tampered token", header: "Bearer " + valid[:20] + "x" + valid[21:]},
		{name: "shorter than the scheme", header: "Bea"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := getWithToken(t, h, "/api/v1/auth/me", tt.header)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if got := apiErrorCode(t, rec); got != "UNAUTHORIZED" {
				t.Errorf("error.code = %q, want UNAUTHORIZED", got)
			}
		})
	}
}

func TestMeAcceptsACaseInsensitiveScheme(t *testing.T) {
	// RFC 7235 makes the scheme case-insensitive.
	h := newAuthAPI(t, newMemStore(), &recordingSender{}, true)
	token := signIn(t, h, "9876543210")

	for _, scheme := range []string{"Bearer ", "bearer ", "BEARER "} {
		t.Run(strings.TrimSpace(scheme), func(t *testing.T) {
			rec := getWithToken(t, h, "/api/v1/auth/me", scheme+token)
			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", rec.Code)
			}
		})
	}
}

func TestMeRejectsATokenForADeletedUser(t *testing.T) {
	store := newMemStore()
	h := newAuthAPI(t, store, &recordingSender{}, true)
	token := signIn(t, h, "9876543210")

	// The account goes away while the token is still cryptographically valid.
	store.users = map[string]auth.User{}

	rec := getWithToken(t, h, "/api/v1/auth/me", "Bearer "+token)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for a stale credential", rec.Code)
	}
}

func TestMeHidesInternalFailures(t *testing.T) {
	store := newMemStore()
	h := newAuthAPI(t, store, &recordingSender{}, true)
	token := signIn(t, h, "9876543210")

	store.userByIDErr = errors.New("connection reset by peer")
	rec := getWithToken(t, h, "/api/v1/auth/me", "Bearer "+token)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "connection reset") {
		t.Errorf("response leaks internals: %s", rec.Body.String())
	}
}

func TestAuthRoutesRejectWrongMethods(t *testing.T) {
	h := newAuthAPI(t, newMemStore(), &recordingSender{}, true)

	tests := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/auth/otp/request"},
		{http.MethodGet, "/api/v1/auth/otp/verify"},
		{http.MethodPost, "/api/v1/auth/me"},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))

			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("status = %d, want 405", rec.Code)
			}
		})
	}
}

func TestMeRejectsAValidlySignedTokenWithANonsenseSubject(t *testing.T) {
	// Correct signature, correct issuer, unexpired — but the subject names no
	// user. The middleware must not let it through.
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   "not-an-object-id",
		Issuer:    "enerzia-api",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}).SignedString(tokenSecret)
	if err != nil {
		t.Fatalf("building the token: %v", err)
	}

	h := newAuthAPI(t, newMemStore(), &recordingSender{}, true)
	rec := getWithToken(t, h, "/api/v1/auth/me", "Bearer "+token)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestUserIDFromWithoutMiddleware(t *testing.T) {
	if _, ok := auth.UserIDFrom(t.Context()); ok {
		t.Error("UserIDFrom(bare ctx) reported a user")
	}
}

// putJSON and postWithToken support the address tests.
func putJSON(t *testing.T, h http.Handler, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	return sendWithToken(t, h, http.MethodPut, path, token, body)
}

func postWithToken(t *testing.T, h http.Handler, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	return sendWithToken(t, h, http.MethodPost, path, token, body)
}

func sendWithToken(t *testing.T, h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func deleteWithToken(t *testing.T, h http.Handler, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	return sendWithToken(t, h, http.MethodDelete, path, token, "")
}
