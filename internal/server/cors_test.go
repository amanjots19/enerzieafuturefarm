package server_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/enerzia/enerzia-be/internal/config"
	"github.com/enerzia/enerzia-be/internal/server"
)

func newCORSServer(t *testing.T, origins ...string) http.Handler {
	t.Helper()
	return server.New(server.Deps{
		Config:  config.Config{AllowedOrigins: origins},
		Mongo:   stubPinger{},
		Logger:  slog.New(slog.NewJSONHandler(io.Discard, nil)),
		Version: "0.1.0",
		Started: time.Now(),
	})
}

func doOrigin(t *testing.T, h http.Handler, method, path, origin string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	h.ServeHTTP(rec, req)
	return rec
}

func doPreflight(t *testing.T, h http.Handler, path, origin, acrm string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, path, nil)
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", acrm)
	h.ServeHTTP(rec, req)
	return rec
}

func TestCORSAllowedOriginEchoedBack(t *testing.T) {
	const origin = "https://shop.example.com"
	h := newCORSServer(t, origin)
	rec := doOrigin(t, h, http.MethodGet, "/health", origin)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
		t.Errorf("ACAO = %q, want %q", got, origin)
	}
}

func TestCORSDisallowedOriginNoHeaderHandlerRan(t *testing.T) {
	h := newCORSServer(t, "https://allowed.example.com")
	rec := doOrigin(t, h, http.MethodGet, "/health", "https://evil.com")

	// 200 proves the health handler ran — the server did not reject with 403.
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (handler must still run)", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("ACAO = %q, want empty for disallowed origin", got)
	}
}

func TestCORSNoOriginPassesThroughUntouched(t *testing.T) {
	h := newCORSServer(t, "https://allowed.example.com")
	// do() from router_test.go sends no Origin header.
	rec := do(t, h, http.MethodGet, "/health")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("ACAO = %q; must be absent when Origin is not sent", got)
	}
	if got := rec.Header().Get("Vary"); got != "" {
		t.Errorf("Vary = %q; must be absent when Origin is not sent", got)
	}
}

func TestCORSPreflightReturns204RouteHandlerNotInvoked(t *testing.T) {
	const origin = "https://shop.example.com"
	h := newCORSServer(t, origin)
	// /health would return 200 with a body if the handler ran; preflight must
	// intercept before the router dispatches.
	rec := doPreflight(t, h, "/health", origin, "GET")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("preflight body = %q; must be empty — route handler must not have run", rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
		t.Errorf("ACAO = %q, want %q", got, origin)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("Access-Control-Allow-Methods must be set on preflight response")
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Error("Access-Control-Allow-Headers must be set on preflight response")
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("Allow-Credentials = %q; must never be set (token is in Authorization header)", got)
	}
}

func TestCORSVaryOriginPresentForAllowedAndDisallowed(t *testing.T) {
	h := newCORSServer(t, "https://shop.example.com")

	for _, tc := range []struct {
		name   string
		origin string
	}{
		{"allowed", "https://shop.example.com"},
		{"disallowed", "https://other.example.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := doOrigin(t, h, http.MethodGet, "/health", tc.origin)
			if got := rec.Header().Get("Vary"); got != "Origin" {
				t.Errorf("Vary = %q, want Origin", got)
			}
		})
	}
}

func TestCORSUnknownPathStillCarriesCORSHeaders(t *testing.T) {
	const origin = "https://shop.example.com"
	h := newCORSServer(t, origin)
	// An unknown path from an allowed cross-origin request must return CORS
	// headers alongside the 404 so the browser can read the error body.
	rec := doOrigin(t, h, http.MethodGet, "/api/v1/does-not-exist", origin)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
		t.Errorf("ACAO = %q, want %q — CORS must cover unknown paths too", got, origin)
	}
}

func TestCORSWildcardNeverEmitted(t *testing.T) {
	h := newCORSServer(t, "https://allowed.example.com")
	for _, origin := range []string{"", "https://evil.com", "*", "null"} {
		rec := doOrigin(t, h, http.MethodGet, "/health", origin)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got == "*" {
			t.Errorf("origin=%q: wildcard emitted — must never happen", origin)
		}
	}
}
