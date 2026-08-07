package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/enerzia/enerzia-be/internal/httpx"
)

// recordHandler records whether it was invoked and returns status.
type recordHandler struct {
	called bool
	status int
}

func (h *recordHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	h.called = true
	w.WriteHeader(h.status)
}

func TestCORSNoOriginPassesThrough(t *testing.T) {
	inner := &recordHandler{status: http.StatusOK}
	h := httpx.CORS([]string{"https://allowed.example.com"})(inner)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if !inner.called {
		t.Fatal("handler was not called for a request with no Origin")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Vary"); got != "" {
		t.Errorf("Vary = %q; must be absent when Origin header is missing", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("ACAO = %q; must be absent when Origin header is missing", got)
	}
}

func TestCORSDisallowedOriginSetsVaryHandlerRuns(t *testing.T) {
	inner := &recordHandler{status: http.StatusOK}
	h := httpx.CORS([]string{"https://allowed.example.com"})(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !inner.called {
		t.Fatal("handler must still run for a disallowed origin")
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary = %q, want Origin", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("ACAO = %q; must not be set for a disallowed origin", got)
	}
}

func TestCORSAllowedOriginEchoedHandlerRuns(t *testing.T) {
	const origin = "https://allowed.example.com"
	inner := &recordHandler{status: http.StatusOK}
	h := httpx.CORS([]string{origin})(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", origin)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !inner.called {
		t.Fatal("handler must run for an allowed origin")
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
		t.Errorf("ACAO = %q, want %q", got, origin)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") == "*" {
		t.Error("wildcard must never be emitted")
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary = %q, want Origin", got)
	}
}

func TestCORSPreflightReturns204HandlerNotCalled(t *testing.T) {
	const origin = "https://allowed.example.com"
	inner := &recordHandler{status: http.StatusOK}
	h := httpx.CORS([]string{origin})(inner)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/orders", nil)
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if inner.called {
		t.Error("route handler must NOT be invoked for a preflight request")
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
		t.Errorf("ACAO = %q, want %q", got, origin)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST, PUT, PATCH, DELETE, OPTIONS" {
		t.Errorf("Allow-Methods = %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Authorization, Content-Type" {
		t.Errorf("Allow-Headers = %q", got)
	}
	if rec.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Error("Allow-Credentials must never be set")
	}
}

func TestCORSWildcardNeverEmittedForAnyInput(t *testing.T) {
	h := httpx.CORS([]string{"https://allowed.example.com"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, origin := range []string{"", "https://evil.com", "*", "null", "https://allowed.example.com"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got == "*" {
			t.Errorf("origin=%q: wildcard emitted", origin)
		}
	}
}
