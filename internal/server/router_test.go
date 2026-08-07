package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/enerzia/enerzia-be/internal/auth"
	"github.com/enerzia/enerzia-be/internal/cart"
	"github.com/enerzia/enerzia-be/internal/catalogue"
	"github.com/enerzia/enerzia-be/internal/config"
	"github.com/enerzia/enerzia-be/internal/httpx"
	"github.com/enerzia/enerzia-be/internal/order"
	"github.com/enerzia/enerzia-be/internal/server"
)

type stubPinger struct{ err error }

func (s stubPinger) Ping(context.Context) error { return s.err }

// newTestServer returns the real router, so these exercise routing, method
// matching and the middleware chain rather than handlers in isolation.
func newTestServer(t *testing.T, pingErr error) http.Handler {
	t.Helper()
	return server.New(server.Deps{
		Config:  config.Config{},
		Mongo:   stubPinger{err: pingErr},
		Logger:  slog.New(slog.NewJSONHandler(io.Discard, nil)),
		Version: "0.1.0",
		Started: time.Now().Add(-time.Minute),
	})
}

func do(t *testing.T, h http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v (%s)", err, rec.Body.String())
	}
	return body.Error.Code
}

func TestHealthRouteIsServed(t *testing.T) {
	rec := do(t, newTestServer(t, nil), http.MethodGet, "/health")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Header().Get(httpx.RequestIDHeader) == "" {
		t.Error("middleware did not run: no request id on the response")
	}
}

func TestHealthReportsDegraded(t *testing.T) {
	rec := do(t, newTestServer(t, errors.New("down")), http.MethodGet, "/health")

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}

func TestUnknownRouteReturnsEnvelopedNotFound(t *testing.T) {
	rec := do(t, newTestServer(t, nil), http.MethodGet, "/api/v1/nope")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if got := errorCode(t, rec); got != string(httpx.CodeNotFound) {
		t.Errorf("error.code = %q, want NOT_FOUND", got)
	}
	if rec.Header().Get(httpx.RequestIDHeader) == "" {
		t.Error("the 404 fallback must also run the middleware chain")
	}
}

func TestWrongMethodReturnsEnvelopedMethodNotAllowed(t *testing.T) {
	rec := do(t, newTestServer(t, nil), http.MethodDelete, "/health")

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	if got := errorCode(t, rec); got != string(httpx.CodeMethodNotAllowed) {
		t.Errorf("error.code = %q, want METHOD_NOT_ALLOWED", got)
	}
	if rec.Header().Get(httpx.RequestIDHeader) == "" {
		t.Error("the 405 fallback must also run the middleware chain")
	}
}

func TestEveryResponseIsJSON(t *testing.T) {
	h := newTestServer(t, nil)
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/health"},
		{http.MethodGet, "/does-not-exist"},
		{http.MethodDelete, "/health"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := do(t, h, tc.method, tc.path)
			if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
				t.Errorf("Content-Type = %q, want JSON", ct)
			}
		})
	}
}

// stubCatalogue registers several routes under /api/v1 so the method-mismatch
// behaviour can be checked on a route that is *not* the last one registered —
// which is the case gorilla/mux gets wrong on its own.
func stubCatalogue(t *testing.T) http.Handler {
	t.Helper()

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	reader := &emptyReader{}
	return server.New(server.Deps{
		Config:    config.Config{},
		Mongo:     stubPinger{},
		Catalogue: catalogue.NewHandler(catalogue.NewService(reader), logger),
		Logger:    logger,
		Version:   "0.1.0",
		Started:   time.Now(),
	})
}

type emptyReader struct{}

func (emptyReader) List(context.Context, catalogue.Form, bool) ([]catalogue.Product, error) {
	return nil, nil
}

func (emptyReader) Get(context.Context, catalogue.ID) (catalogue.Product, error) {
	return catalogue.Product{}, catalogue.ErrProductNotFound
}

func (emptyReader) Siblings(context.Context, catalogue.Family, catalogue.ID) ([]catalogue.Product, error) {
	return nil, nil
}

func (emptyReader) TrustTiles(context.Context) ([]catalogue.TrustTile, error) { return nil, nil }

func TestWrongMethodIs405EvenOnAnEarlyRegisteredRoute(t *testing.T) {
	h := stubCatalogue(t)

	// /products is registered before two other routes; mux alone would answer
	// 404 here because a later path mismatch clears the method-mismatch flag.
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/products"},
		{http.MethodDelete, "/api/v1/products"},
		{http.MethodPut, "/api/v1/products/tablets-120"},
		{http.MethodPost, "/api/v1/content/trust"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := do(t, h, tc.method, tc.path)

			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, want 405", rec.Code)
			}
			if got := errorCode(t, rec); got != string(httpx.CodeMethodNotAllowed) {
				t.Errorf("error.code = %q, want METHOD_NOT_ALLOWED", got)
			}
		})
	}
}

func TestUnknownPathUnderAPIPrefixIsStill404(t *testing.T) {
	// The 405 recovery must not turn every unmatched path into a 405.
	h := stubCatalogue(t)

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/nope"},
		{http.MethodPost, "/api/v1/nope"},
		{http.MethodGet, "/api/v1/products/tablets-120/extra"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := do(t, h, tc.method, tc.path)

			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404", rec.Code)
			}
			if got := errorCode(t, rec); got != string(httpx.CodeNotFound) {
				t.Errorf("error.code = %q, want NOT_FOUND", got)
			}
		})
	}
}

func TestAPIPrefixConstant(t *testing.T) {
	if server.APIPrefix != "/api/v1" {
		t.Errorf("APIPrefix = %q, want /api/v1 as roadmap.md specifies", server.APIPrefix)
	}
}

// nullParser always rejects tokens so auth middleware returns 401 without
// calling any service method.
type nullParser struct{}

func (nullParser) ParseToken(string) (auth.Claims, error) {
	return auth.Claims{}, errors.New("no token")
}

func TestAllHandlersAreRegistered(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	// Minimal handler instances — none of the underlying service methods are
	// called; we only need the handlers to be non-nil so Register is invoked.
	authSvc := auth.NewService(auth.ServiceConfig{})
	cartSvc := cart.NewService(nil, nil)
	orderSvc := order.NewService(order.ServiceConfig{Logger: logger})
	h := server.New(server.Deps{
		Config:  config.Config{},
		Mongo:   stubPinger{},
		Auth:    auth.NewHandler(authSvc, logger),
		Cart:    cart.NewHandler(cartSvc, nullParser{}, logger),
		Order:   order.NewHandler(orderSvc, nullParser{}, logger),
		Logger:  logger,
		Version: "0.1.0",
		Started: time.Now(),
	})
	// POST /api/v1/orders: auth middleware returns 401, not 404.
	rec := do(t, h, http.MethodPost, "/api/v1/orders")
	if rec.Code == http.StatusNotFound {
		t.Error("POST /api/v1/orders was not registered")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("order route: expected 401 from auth middleware, got %d", rec.Code)
	}
	// POST /api/v1/auth/otp/request: exists (not 404).
	rec = do(t, h, http.MethodPost, "/api/v1/auth/otp/request")
	if rec.Code == http.StatusNotFound {
		t.Error("POST /api/v1/auth/otp/request was not registered")
	}
	// GET /api/v1/cart: requires auth → 401.
	rec = do(t, h, http.MethodGet, "/api/v1/cart")
	if rec.Code == http.StatusNotFound {
		t.Error("GET /api/v1/cart was not registered")
	}
	// GET /api/v1/orders: auth middleware returns 401, not 404.
	rec = do(t, h, http.MethodGet, "/api/v1/orders")
	if rec.Code == http.StatusNotFound {
		t.Error("GET /api/v1/orders was not registered")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("GET /api/v1/orders: expected 401 from auth middleware, got %d", rec.Code)
	}
	// GET /api/v1/orders/{orderId}: auth middleware returns 401, not 404.
	rec = do(t, h, http.MethodGet, "/api/v1/orders/EFF-000001")
	if rec.Code == http.StatusNotFound {
		t.Error("GET /api/v1/orders/{orderId} was not registered")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("GET /api/v1/orders/{orderId}: expected 401 from auth middleware, got %d", rec.Code)
	}
	// POST /webhooks/razorpay: on root (outside /api/v1 and outside auth).
	// No signature → 400 BAD_REQUEST, not 404.
	rec = do(t, h, http.MethodPost, "/webhooks/razorpay")
	if rec.Code == http.StatusNotFound {
		t.Error("POST /webhooks/razorpay was not registered")
	}
}
