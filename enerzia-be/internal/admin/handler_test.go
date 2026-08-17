package admin_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/enerzia/enerzia-be/internal/admin"
	"github.com/enerzia/enerzia-be/internal/config"
	"github.com/enerzia/enerzia-be/internal/server"
)

const (
	testAdminEmail    = "ops@enerzia.in"
	testAdminPassword = "correct-admin-password-long-enough"
)

// testPasswordHash is a bcrypt hash of testAdminPassword at MinCost so tests
// do not spend 60 ms per login call.
var testPasswordHash = func() string {
	h, err := bcrypt.GenerateFromPassword([]byte(testAdminPassword), bcrypt.MinCost)
	if err != nil {
		panic("handler_test: bcrypt setup: " + err.Error())
	}
	return string(h)
}()

type stubPinger struct{}

func (stubPinger) Ping(context.Context) error { return nil }

// newAdminAPI builds the real router with only the admin handler wired in.
func newAdminAPI(t *testing.T, email, passwordHash string) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	tokens := admin.NewTokenIssuer(adminSecret, admin.TokenTTL)
	svc := admin.NewService(admin.ServiceConfig{
		Email:        email,
		PasswordHash: passwordHash,
		Tokens:       tokens,
		Limiter:      admin.NewLimiter(),
	})
	return server.New(server.Deps{
		Config:  config.Config{},
		Mongo:   stubPinger{},
		Admin:   admin.NewHandler(svc, logger),
		Logger:  logger,
		Version: "test",
		Started: time.Now(),
	})
}

func doAdmin(t *testing.T, h http.Handler, method, path, body, remoteAddr, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	if remoteAddr != "" {
		req.RemoteAddr = remoteAddr
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func adminErrorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("body is not valid JSON: %v (%s)", err, rec.Body.String())
	}
	return env.Error.Code
}

func adminErrorMessage(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("body is not valid JSON: %v (%s)", err, rec.Body.String())
	}
	return env.Error.Message
}

func firstFieldError(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var resp struct {
		Error struct {
			Details struct {
				Fields []struct {
					Field string `json:"field"`
				} `json:"fields"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("body is not valid JSON: %v (%s)", err, rec.Body.String())
	}
	if len(resp.Error.Details.Fields) == 0 {
		return ""
	}
	return resp.Error.Details.Fields[0].Field
}

/* --------------------------------------------------------------- login tests */

func TestLoginSuccess(t *testing.T) {
	h := newAdminAPI(t, testAdminEmail, testPasswordHash)
	body := fmt.Sprintf(`{"email":%q,"password":%q}`, testAdminEmail, testAdminPassword)

	rec := doAdmin(t, h, http.MethodPost, "/api/v1/admin/login", body, "10.0.0.1:1234", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}

	var resp struct {
		Data struct {
			Token     string    `json:"token"`
			ExpiresAt time.Time `json:"expiresAt"`
			Email     string    `json:"email"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if resp.Data.Token == "" {
		t.Error("token is empty")
	}
	if resp.Data.ExpiresAt.IsZero() {
		t.Error("expiresAt is zero")
	}
	// The canonical email (testAdminEmail) must be returned, not body.Email.
	if resp.Data.Email != testAdminEmail {
		t.Errorf("email = %q, want canonical %q", resp.Data.Email, testAdminEmail)
	}
}

func TestLoginBlankEmailIs422(t *testing.T) {
	h := newAdminAPI(t, testAdminEmail, testPasswordHash)
	body := fmt.Sprintf(`{"email":"","password":%q}`, testAdminPassword)

	rec := doAdmin(t, h, http.MethodPost, "/api/v1/admin/login", body, "10.0.0.1:1234", "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body: %s", rec.Code, rec.Body)
	}
	if got := firstFieldError(t, rec); got != "email" {
		t.Errorf("first field error = %q, want %q", got, "email")
	}
}

func TestLoginBlankPasswordIs422(t *testing.T) {
	h := newAdminAPI(t, testAdminEmail, testPasswordHash)
	body := fmt.Sprintf(`{"email":%q,"password":""}`, testAdminEmail)

	rec := doAdmin(t, h, http.MethodPost, "/api/v1/admin/login", body, "10.0.0.1:1234", "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body: %s", rec.Code, rec.Body)
	}
	// Email is present, so the first error must be password.
	if got := firstFieldError(t, rec); got != "password" {
		t.Errorf("first field error = %q, want %q", got, "password")
	}
}

func TestLoginWrongEmailIs401WithCorrectMessage(t *testing.T) {
	h := newAdminAPI(t, testAdminEmail, testPasswordHash)
	body := fmt.Sprintf(`{"email":"wrong@example.com","password":%q}`, testAdminPassword)

	rec := doAdmin(t, h, http.MethodPost, "/api/v1/admin/login", body, "10.0.0.1:1234", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body: %s", rec.Code, rec.Body)
	}
	const want = "Email or password is incorrect."
	if got := adminErrorMessage(t, rec); got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

func TestLoginWrongPasswordIs401WithCorrectMessage(t *testing.T) {
	h := newAdminAPI(t, testAdminEmail, testPasswordHash)
	body := fmt.Sprintf(`{"email":%q,"password":"wrongpassword"}`, testAdminEmail)

	rec := doAdmin(t, h, http.MethodPost, "/api/v1/admin/login", body, "10.0.0.1:1234", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body: %s", rec.Code, rec.Body)
	}
	const want = "Email or password is incorrect."
	if got := adminErrorMessage(t, rec); got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

func TestLoginUnconfiguredIs401(t *testing.T) {
	// No admin email/hash — service returns ErrNotConfigured, still maps to 401.
	h := newAdminAPI(t, "", "")
	body := `{"email":"anyone@example.com","password":"anypassword1234"}`

	rec := doAdmin(t, h, http.MethodPost, "/api/v1/admin/login", body, "10.0.0.1:1234", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for unconfigured admin; body: %s", rec.Code, rec.Body)
	}
	// Message must be identical to wrong-credentials — callers cannot tell
	// whether an admin account is configured.
	const want = "Email or password is incorrect."
	if got := adminErrorMessage(t, rec); got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

func TestLoginUnconfiguredRateLimitedAfter10Failures(t *testing.T) {
	// 11 attempts against an unconfigured service must produce a 429, not a
	// stream of 401s, so the configured and unconfigured states are
	// indistinguishable from outside.
	h := newAdminAPI(t, "", "")
	const ip = "10.0.0.1:1234"
	wrongBody := `{"email":"anyone@example.com","password":"anypassword1234"}`

	for i := range 10 {
		rec := doAdmin(t, h, http.MethodPost, "/api/v1/admin/login", wrongBody, ip, "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i+1, rec.Code)
		}
	}

	rec := doAdmin(t, h, http.MethodPost, "/api/v1/admin/login", wrongBody, ip, "")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("11th attempt (unconfigured): status = %d, want 429; body: %s", rec.Code, rec.Body)
	}
}

func TestLoginCaseInsensitiveEmail(t *testing.T) {
	h := newAdminAPI(t, testAdminEmail, testPasswordHash)
	body := fmt.Sprintf(`{"email":%q,"password":%q}`,
		strings.ToUpper(testAdminEmail), testAdminPassword)

	rec := doAdmin(t, h, http.MethodPost, "/api/v1/admin/login", body, "10.0.0.1:1234", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("case-insensitive login failed: status = %d, body: %s", rec.Code, rec.Body)
	}
}

func TestLoginRateLimitedAfter10Failures(t *testing.T) {
	h := newAdminAPI(t, testAdminEmail, testPasswordHash)
	const ip = "10.0.0.1:1234"
	wrongBody := fmt.Sprintf(`{"email":%q,"password":"wrongpassword"}`, testAdminEmail)

	for i := range 10 {
		rec := doAdmin(t, h, http.MethodPost, "/api/v1/admin/login", wrongBody, ip, "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i+1, rec.Code)
		}
	}

	rec := doAdmin(t, h, http.MethodPost, "/api/v1/admin/login", wrongBody, ip, "")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("11th attempt: status = %d, want 429; body: %s", rec.Code, rec.Body)
	}
	const want = "Too many sign-in attempts. Try again later."
	if got := adminErrorMessage(t, rec); got != want {
		t.Errorf("rate-limit message = %q, want %q", got, want)
	}
}

func TestLoginRateLimitDoesNotAffectDifferentIP(t *testing.T) {
	h := newAdminAPI(t, testAdminEmail, testPasswordHash)
	const ipA = "10.0.0.1:1234"
	const ipB = "10.0.0.2:1234"
	wrongBody := fmt.Sprintf(`{"email":%q,"password":"wrongpassword"}`, testAdminEmail)

	for range 10 {
		doAdmin(t, h, http.MethodPost, "/api/v1/admin/login", wrongBody, ipA, "")
	}

	// ipB has not hit the limit; it should see 401 (wrong password), not 429.
	rec := doAdmin(t, h, http.MethodPost, "/api/v1/admin/login", wrongBody, ipB, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("ipB status = %d after ipA exhausted, want 401 (IPs are isolated)", rec.Code)
	}
}

func TestLoginWrongMethod(t *testing.T) {
	h := newAdminAPI(t, testAdminEmail, testPasswordHash)
	rec := doAdmin(t, h, http.MethodGet, "/api/v1/admin/login", "", "10.0.0.1:1234", "")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /admin/login status = %d, want 405", rec.Code)
	}
	// Must be the standard envelope, not gorilla's plain-text default.
	if got := adminErrorCode(t, rec); got != "METHOD_NOT_ALLOWED" {
		t.Errorf("error code = %q, want METHOD_NOT_ALLOWED", got)
	}
}

func TestUnknownAdminPathIs404InEnvelope(t *testing.T) {
	h := newAdminAPI(t, testAdminEmail, testPasswordHash)
	// /admin/mexyz must not match /admin/me — it is a 404, not a 405.
	rec := doAdmin(t, h, http.MethodGet, "/api/v1/admin/mexyz", "", "10.0.0.1:1234", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /admin/mexyz status = %d, want 404", rec.Code)
	}
	if got := adminErrorCode(t, rec); got != "NOT_FOUND" {
		t.Errorf("error code = %q, want NOT_FOUND", got)
	}
}

/* ---------------------------------------------------------------- /me tests */

func loginAndGetToken(t *testing.T, h http.Handler) string {
	t.Helper()
	body := fmt.Sprintf(`{"email":%q,"password":%q}`, testAdminEmail, testAdminPassword)
	rec := doAdmin(t, h, http.MethodPost, "/api/v1/admin/login", body, "10.0.0.1:1234", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("loginAndGetToken: status = %d; body: %s", rec.Code, rec.Body)
	}
	var resp struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("loginAndGetToken: %v", err)
	}
	return resp.Data.Token
}

func TestMeWithValidToken(t *testing.T) {
	h := newAdminAPI(t, testAdminEmail, testPasswordHash)
	token := loginAndGetToken(t, h)

	rec := doAdmin(t, h, http.MethodGet, "/api/v1/admin/me", "", "10.0.0.1:1234", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /admin/me status = %d, want 200; body: %s", rec.Code, rec.Body)
	}

	var resp struct {
		Data struct {
			Email string `json:"email"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("body is not valid JSON: %v", err)
	}
	if resp.Data.Email != testAdminEmail {
		t.Errorf("email = %q, want %q", resp.Data.Email, testAdminEmail)
	}
}

func TestMeWithoutTokenIs401(t *testing.T) {
	h := newAdminAPI(t, testAdminEmail, testPasswordHash)
	rec := doAdmin(t, h, http.MethodGet, "/api/v1/admin/me", "", "10.0.0.1:1234", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /admin/me (no token): status = %d, want 401", rec.Code)
	}
	// Envelope must be standard JSON, not plain text.
	if got := adminErrorCode(t, rec); got != "UNAUTHORIZED" {
		t.Errorf("error code = %q, want UNAUTHORIZED", got)
	}
}

func TestMeWithCustomerTokenIs401(t *testing.T) {
	// A token with issuer "enerzia-api" (the customer issuer) must NOT be
	// accepted by the admin middleware, even when signed with the same secret.
	h := newAdminAPI(t, testAdminEmail, testPasswordHash)

	customerToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   "000000000000000000000001",
		"phone": "9876543210",
		"iss":   "enerzia-api",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}).SignedString(adminSecret)
	if err != nil {
		t.Fatalf("build customer token: %v", err)
	}

	rec := doAdmin(t, h, http.MethodGet, "/api/v1/admin/me", "", "10.0.0.1:1234", customerToken)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /admin/me (customer token): status = %d, want 401", rec.Code)
	}
}

func TestMeWrongMethod(t *testing.T) {
	h := newAdminAPI(t, testAdminEmail, testPasswordHash)
	token := loginAndGetToken(t, h)
	// POST to /admin/me is not registered — must be 405 in the standard envelope.
	rec := doAdmin(t, h, http.MethodPost, "/api/v1/admin/me", "{}", "10.0.0.1:1234", token)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /admin/me status = %d, want 405", rec.Code)
	}
	if got := adminErrorCode(t, rec); got != "METHOD_NOT_ALLOWED" {
		t.Errorf("error code = %q, want METHOD_NOT_ALLOWED", got)
	}
}

/* -------------------------------------------------- additional edge cases */

func TestLoginBadJSONBodyIs400(t *testing.T) {
	h := newAdminAPI(t, testAdminEmail, testPasswordHash)
	// Not valid JSON — exercises the decodeAdminJSON error path.
	rec := doAdmin(t, h, http.MethodPost, "/api/v1/admin/login", "not-json", "10.0.0.1:1234", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for bad JSON; body: %s", rec.Code, rec.Body)
	}
	if got := adminErrorCode(t, rec); got != "BAD_REQUEST" {
		t.Errorf("error code = %q, want BAD_REQUEST", got)
	}
}

func TestLoginUnknownJSONFieldIs400(t *testing.T) {
	h := newAdminAPI(t, testAdminEmail, testPasswordHash)
	body := `{"email":"x@x.com","password":"y","unknown":"z"}`
	rec := doAdmin(t, h, http.MethodPost, "/api/v1/admin/login", body, "10.0.0.1:1234", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for unknown field; body: %s", rec.Code, rec.Body)
	}
}

func TestLoginRemoteAddrWithoutPort(t *testing.T) {
	// When RemoteAddr has no port, net.SplitHostPort fails and remoteIP falls
	// back to the raw address. Login should still succeed.
	h := newAdminAPI(t, testAdminEmail, testPasswordHash)
	body := fmt.Sprintf(`{"email":%q,"password":%q}`, testAdminEmail, testAdminPassword)
	rec := doAdmin(t, h, http.MethodPost, "/api/v1/admin/login", body, "10.0.0.1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("login with addr-without-port: status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
}

func TestMeWrongBearerScheme(t *testing.T) {
	// "Basic ..." is not a bearer token; the middleware must reject it via the
	// case-insensitive scheme check.
	h := newAdminAPI(t, testAdminEmail, testPasswordHash)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/me", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("Authorization", "Basic c29tZXRoaW5n")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /admin/me (Basic auth): status = %d, want 401", rec.Code)
	}
	if got := adminErrorCode(t, rec); got != "UNAUTHORIZED" {
		t.Errorf("error code = %q, want UNAUTHORIZED", got)
	}
}
