package order_test

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

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/enerzia/enerzia-be/internal/auth"
	"github.com/enerzia/enerzia-be/internal/cart"
	"github.com/enerzia/enerzia-be/internal/catalogue"
	"github.com/enerzia/enerzia-be/internal/config"
	"github.com/enerzia/enerzia-be/internal/order"
	"github.com/enerzia/enerzia-be/internal/razorpay"
	"github.com/enerzia/enerzia-be/internal/server"
)

/* ================================================================ helpers */

var orderSecret = []byte("order-test-secret-at-least-32-chars!")

type realParser struct{ issuer *auth.TokenIssuer }

func (p realParser) ParseToken(token string) (auth.Claims, error) { return p.issuer.Parse(token) }

type stubMongoPinger struct{}

func (stubMongoPinger) Ping(context.Context) error { return nil }

// newOrderAPI wires fakes through the real service, handler, auth middleware
// and router, then returns a valid bearer token for testUser. An optional
// CartClearer can be passed as the last argument for callback tests.
func newOrderAPI(
	t *testing.T,
	store order.Store,
	liner order.CartLiner,
	addr order.AddressResolver,
	stock order.StockKeeper,
	gw razorpay.Gateway,
	clearer ...order.CartClearer,
) (http.Handler, string) {
	t.Helper()

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	issuer := auth.NewTokenIssuer(orderSecret, auth.TokenTTL)

	token, _, err := issuer.Issue(auth.User{ID: testUser, Phone: "9876543210"})
	if err != nil {
		t.Fatalf("issuing test token: %v", err)
	}

	var c order.CartClearer
	if len(clearer) > 0 {
		c = clearer[0]
	}

	svc := order.NewService(order.ServiceConfig{
		Repo:          store,
		Cart:          liner,
		Auth:          addr,
		Catalogue:     stock,
		Gateway:       gw,
		Events:        &fakeEventStore{},
		RazorpayKeyID: "rzp_test_KEY",
		CartClearer:   c,
		Logger:        logger,
		Now:           func() time.Time { return fixedNow },
	})

	h := server.New(server.Deps{
		Config:  config.Config{},
		Mongo:   stubMongoPinger{},
		Order:   order.NewHandler(svc, realParser{issuer}, logger),
		Logger:  logger,
		Version: "test",
		Started: time.Now(),
	})
	return h, token
}

// goodAPILine returns a non-blocking cart line for handler tests.
func goodAPILine() cart.Line {
	return cart.Line{
		ProductID: catalogue.ID("prod-powder"),
		Name:      "Spirulina Powder 100g",
		Form:      catalogue.FormPowder,
		Grad:      "#4ade80",
		UnitPrice: 49900,
		UnitMRP:   59900,
		Qty:       2,
		LineTotal: 99800,
		Stock:     50,
	}
}

var defaultAddr = auth.Address{
	ID:        bson.NewObjectID(),
	Name:      "Test User",
	Email:     "test@example.com",
	Line1:     "123 Main Street",
	City:      "Mumbai",
	State:     "Maharashtra",
	Pin:       "400001",
	IsDefault: true,
}

func doOrder(t *testing.T, h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func errCode(rec *httptest.ResponseRecorder) string {
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return body.Error.Code
}

/* ================================================================ auth */

func TestOrderRouteRequiresAuth(t *testing.T) {
	h, _ := newOrderAPI(t, &fakeStore{}, &fakeLiner{}, &fakeAddr{addr: defaultAddr}, newFakeStock(), &fakeGateway{})
	rec := doOrder(t, h, http.MethodPost, "/api/v1/orders", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

/* ================================================================ success */

func TestOpenCheckoutSuccess(t *testing.T) {
	lines := []cart.Line{goodAPILine()}
	gw := &fakeGateway{rzpOrder: razorpay.Order{ID: "order_abc123", Amount: 99800, Currency: "INR"}}

	h, token := newOrderAPI(t, &fakeStore{}, &fakeLiner{lines: lines}, &fakeAddr{addr: defaultAddr}, newFakeStock(), gw)
	rec := doOrder(t, h, http.MethodPost, "/api/v1/orders", token, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (%s)", rec.Code, rec.Body.String())
	}

	var body struct {
		Data struct {
			Order struct {
				OrderID   string `json:"orderId"`
				Status    string `json:"status"`
				ExpiresAt string `json:"expiresAt"`
				Lines     []struct {
					ProductID string `json:"productId"`
					Qty       int    `json:"qty"`
					LineTotal int64  `json:"lineTotal"`
				} `json:"lines"`
				Totals struct {
					Total int64 `json:"total"`
				} `json:"totals"`
			} `json:"order"`
			Razorpay struct {
				KeyID           string `json:"keyId"`
				RazorpayOrderID string `json:"razorpayOrderId"`
				Amount          int64  `json:"amount"`
				Currency        string `json:"currency"`
			} `json:"razorpay"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not JSON: %v (%s)", err, rec.Body.String())
	}

	d := body.Data
	if d.Order.Status != "pending_payment" {
		t.Errorf("status = %q, want pending_payment", d.Order.Status)
	}
	if d.Razorpay.KeyID != "rzp_test_KEY" {
		t.Errorf("keyId = %q, want rzp_test_KEY", d.Razorpay.KeyID)
	}
	if d.Razorpay.RazorpayOrderID != "order_abc123" {
		t.Errorf("razorpayOrderId = %q, want order_abc123", d.Razorpay.RazorpayOrderID)
	}
	if d.Razorpay.Amount != 99800 {
		t.Errorf("amount = %d, want 99800", d.Razorpay.Amount)
	}
	if d.Razorpay.Currency != "INR" {
		t.Errorf("currency = %q, want INR", d.Razorpay.Currency)
	}
	if len(d.Order.Lines) != 1 {
		t.Errorf("lines = %d, want 1", len(d.Order.Lines))
	}
}

func TestOpenCheckoutEmptyBodyIsValid(t *testing.T) {
	lines := []cart.Line{goodAPILine()}
	gw := &fakeGateway{rzpOrder: razorpay.Order{ID: "order_empty", Amount: 99800, Currency: "INR"}}
	h, token := newOrderAPI(t, &fakeStore{}, &fakeLiner{lines: lines}, &fakeAddr{addr: defaultAddr}, newFakeStock(), gw)

	// Send a completely empty body (no Content-Type header set deliberately to test EOF path)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201 (empty body must be valid): %s", rec.Code, rec.Body.String())
	}
}

func TestOpenCheckoutWithAddressID(t *testing.T) {
	lines := []cart.Line{goodAPILine()}
	addrID := defaultAddr.ID
	gw := &fakeGateway{rzpOrder: razorpay.Order{ID: "order_with_addr", Amount: 99800, Currency: "INR"}}
	h, token := newOrderAPI(t, &fakeStore{}, &fakeLiner{lines: lines}, &fakeAddr{addr: defaultAddr}, newFakeStock(), gw)

	body := `{"addressId":"` + addrID.Hex() + `"}`
	rec := doOrder(t, h, http.MethodPost, "/api/v1/orders", token, body)
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
}

/* ================================================================ errors */

func TestOpenCheckoutEmptyCart(t *testing.T) {
	h, token := newOrderAPI(t, &fakeStore{}, &fakeLiner{}, &fakeAddr{addr: defaultAddr}, newFakeStock(), &fakeGateway{})
	rec := doOrder(t, h, http.MethodPost, "/api/v1/orders", token, "")
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
	if c := errCode(rec); c != "CONFLICT" {
		t.Errorf("code = %q, want CONFLICT", c)
	}
}

func TestOpenCheckoutBlockingLine(t *testing.T) {
	blocking := cart.Line{
		ProductID: "prod-x",
		Name:      "Spirulina Powder 100g",
		Form:      catalogue.FormPowder,
		Qty:       5,
		Stock:     2,
		UnitPrice: 49900,
		LineTotal: 249500,
	}
	h, token := newOrderAPI(t, &fakeStore{}, &fakeLiner{lines: []cart.Line{blocking}}, &fakeAddr{addr: defaultAddr}, newFakeStock(), &fakeGateway{})
	rec := doOrder(t, h, http.MethodPost, "/api/v1/orders", token, "")
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
}

func TestOpenCheckoutNoSavedAddress(t *testing.T) {
	lines := []cart.Line{goodAPILine()}
	h, token := newOrderAPI(t, &fakeStore{}, &fakeLiner{lines: lines}, &fakeAddr{err: auth.ErrAddressNotFound}, newFakeStock(), &fakeGateway{})
	rec := doOrder(t, h, http.MethodPost, "/api/v1/orders", token, "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422 (no saved address)", rec.Code)
	}
}

func TestOpenCheckoutAddressNotFound(t *testing.T) {
	lines := []cart.Line{goodAPILine()}
	h, token := newOrderAPI(t, &fakeStore{}, &fakeLiner{lines: lines}, &fakeAddr{err: auth.ErrAddressNotFound}, newFakeStock(), &fakeGateway{})

	addrHex := bson.NewObjectID().Hex()
	rec := doOrder(t, h, http.MethodPost, "/api/v1/orders", token, `{"addressId":"`+addrHex+`"}`)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (address not found)", rec.Code)
	}
}

func TestOpenCheckoutPendingOrderExists(t *testing.T) {
	lines := []cart.Line{goodAPILine()}
	store := &fakeStore{createErr: order.ErrPendingOrderExists}
	h, token := newOrderAPI(t, store, &fakeLiner{lines: lines}, &fakeAddr{addr: defaultAddr}, newFakeStock(), &fakeGateway{})
	rec := doOrder(t, h, http.MethodPost, "/api/v1/orders", token, "")
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
}

func TestOpenCheckoutStockUnavailable(t *testing.T) {
	lines := []cart.Line{goodAPILine()}
	stock := newFakeStock()
	stock.failOnIdx = 0
	stock.takeErr = catalogue.ErrOutOfStock
	h, token := newOrderAPI(t, &fakeStore{}, &fakeLiner{lines: lines}, &fakeAddr{addr: defaultAddr}, stock, &fakeGateway{})
	rec := doOrder(t, h, http.MethodPost, "/api/v1/orders", token, "")
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409 (stock unavailable)", rec.Code)
	}
}

func TestOpenCheckoutGatewayFailure(t *testing.T) {
	lines := []cart.Line{goodAPILine()}
	gw := &fakeGateway{err: razorpay.ErrNotConfigured}
	h, token := newOrderAPI(t, &fakeStore{}, &fakeLiner{lines: lines}, &fakeAddr{addr: defaultAddr}, newFakeStock(), gw)
	rec := doOrder(t, h, http.MethodPost, "/api/v1/orders", token, "")
	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
	if c := errCode(rec); c != "GATEWAY_ERROR" {
		t.Errorf("code = %q, want GATEWAY_ERROR", c)
	}
}

func TestOpenCheckoutBadAddressIDShape(t *testing.T) {
	lines := []cart.Line{goodAPILine()}
	h, token := newOrderAPI(t, &fakeStore{}, &fakeLiner{lines: lines}, &fakeAddr{addr: defaultAddr}, newFakeStock(), &fakeGateway{})
	rec := doOrder(t, h, http.MethodPost, "/api/v1/orders", token, `{"addressId":"not-a-valid-objectid"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422 (invalid addressId)", rec.Code)
	}
}

func TestOpenCheckoutUnknownFieldsRejected(t *testing.T) {
	lines := []cart.Line{goodAPILine()}
	h, token := newOrderAPI(t, &fakeStore{}, &fakeLiner{lines: lines}, &fakeAddr{addr: defaultAddr}, newFakeStock(), &fakeGateway{})
	rec := doOrder(t, h, http.MethodPost, "/api/v1/orders", token, `{"unknownField":"value"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (unknown field rejected)", rec.Code)
	}
}

func TestOrderResponseIsJSON(t *testing.T) {
	lines := []cart.Line{goodAPILine()}
	gw := &fakeGateway{rzpOrder: razorpay.Order{ID: "order_json", Amount: 99800, Currency: "INR"}}
	h, token := newOrderAPI(t, &fakeStore{}, &fakeLiner{lines: lines}, &fakeAddr{addr: defaultAddr}, newFakeStock(), gw)
	rec := doOrder(t, h, http.MethodPost, "/api/v1/orders", token, "")
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", ct)
	}
}

func TestOrderRouteRejectsWrongMethod(t *testing.T) {
	h, token := newOrderAPI(t, &fakeStore{}, &fakeLiner{}, &fakeAddr{addr: defaultAddr}, newFakeStock(), &fakeGateway{})
	// PUT is not a registered method on /api/v1/orders — expect 405.
	rec := doOrder(t, h, http.MethodPut, "/api/v1/orders", token, "")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

/* ================================================================ paymentCallback */

const callbackPath = "/api/v1/orders/EFF-000001/payment/callback"

// validCallbackBody returns a well-formed callback body where razorpayOrderId
// matches what fakeStore.byIDOrder carries.
func validCallbackBody() string {
	return `{"razorpayOrderId":"order_rz","razorpayPaymentId":"pay_rz","razorpaySignature":"sig_rz"}`
}

// pendingAPIOrder returns a fakeStore pre-loaded with a pending_payment order
// whose razorpayOrderId is "order_rz".
func pendingAPIStore() *fakeStore {
	return &fakeStore{
		byIDOrder:    order.Order{OrderID: "EFF-000001", Status: order.StatusPendingPayment, Payment: order.Payment{Provider: "razorpay", Status: order.PaymentStatusCreated, Amount: 49900, Currency: "INR", RazorpayOrderID: "order_rz"}},
		markModified: true,
	}
}

func TestPaymentCallbackRequiresAuth(t *testing.T) {
	h, _ := newOrderAPI(t, &fakeStore{}, &fakeLiner{}, &fakeAddr{addr: defaultAddr}, newFakeStock(), &fakeGateway{})
	rec := doOrder(t, h, http.MethodPost, callbackPath, "", validCallbackBody())
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestPaymentCallbackSuccess(t *testing.T) {
	clearer := &fakeCartClearer{}
	h, token := newOrderAPI(t, pendingAPIStore(), &fakeLiner{}, &fakeAddr{addr: defaultAddr}, newFakeStock(), &fakeGateway{}, clearer)
	rec := doOrder(t, h, http.MethodPost, callbackPath, token, validCallbackBody())
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}

	var body struct {
		Data struct {
			Order struct {
				Status   string `json:"status"`
				PlacedAt string `json:"placedAt"`
				EtaText  string `json:"etaText"`
				Payment  struct {
					Method interface{} `json:"method"`
					Last4  interface{} `json:"last4"`
				} `json:"payment"`
			} `json:"order"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not JSON: %v (%s)", err, rec.Body.String())
	}
	o := body.Data.Order
	if o.Status != "placed" {
		t.Errorf("status = %q, want placed", o.Status)
	}
	if o.PlacedAt == "" {
		t.Error("placedAt should be set")
	}
	if o.EtaText == "" {
		t.Error("etaText should be set")
	}
	if !clearer.cleared {
		t.Error("cart was not cleared")
	}
}

func TestPaymentCallbackOrderNotFound(t *testing.T) {
	store := &fakeStore{byIDErr: order.ErrOrderNotFound}
	h, token := newOrderAPI(t, store, &fakeLiner{}, &fakeAddr{addr: defaultAddr}, newFakeStock(), &fakeGateway{})
	rec := doOrder(t, h, http.MethodPost, callbackPath, token, validCallbackBody())
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestPaymentCallbackSignatureInvalid(t *testing.T) {
	gw := &fakeGateway{verifyCBErr: errors.New("hmac mismatch")}
	h, token := newOrderAPI(t, pendingAPIStore(), &fakeLiner{}, &fakeAddr{addr: defaultAddr}, newFakeStock(), gw)
	rec := doOrder(t, h, http.MethodPost, callbackPath, token, validCallbackBody())
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", rec.Code)
	}
	if c := errCode(rec); c != "VALIDATION_ERROR" {
		t.Errorf("code = %q, want VALIDATION_ERROR", c)
	}
}

func TestPaymentCallbackOrderNotPending(t *testing.T) {
	store := &fakeStore{byIDOrder: order.Order{OrderID: "EFF-000001", Status: order.StatusExpired}}
	h, token := newOrderAPI(t, store, &fakeLiner{}, &fakeAddr{addr: defaultAddr}, newFakeStock(), &fakeGateway{})
	rec := doOrder(t, h, http.MethodPost, callbackPath, token, validCallbackBody())
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422 (order not pending)", rec.Code)
	}
}

func TestPaymentCallbackEmptyBody(t *testing.T) {
	h, token := newOrderAPI(t, pendingAPIStore(), &fakeLiner{}, &fakeAddr{addr: defaultAddr}, newFakeStock(), &fakeGateway{})
	rec := doOrder(t, h, http.MethodPost, callbackPath, token, "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422 (missing required fields)", rec.Code)
	}
}

func TestPaymentCallbackMissingFields(t *testing.T) {
	h, token := newOrderAPI(t, pendingAPIStore(), &fakeLiner{}, &fakeAddr{addr: defaultAddr}, newFakeStock(), &fakeGateway{})
	rec := doOrder(t, h, http.MethodPost, callbackPath, token, `{"razorpayOrderId":"order_rz"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422 (missing fields)", rec.Code)
	}
}

func TestPaymentCallbackAllFieldsMissing(t *testing.T) {
	// Empty JSON object: all three required fields are absent, covering the
	// razorpayOrderId == "" append branch that TestPaymentCallbackMissingFields
	// does not reach (it supplies a non-empty razorpayOrderId).
	h, token := newOrderAPI(t, pendingAPIStore(), &fakeLiner{}, &fakeAddr{addr: defaultAddr}, newFakeStock(), &fakeGateway{})
	rec := doOrder(t, h, http.MethodPost, callbackPath, token, `{}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422 (all fields missing)", rec.Code)
	}
}

func TestPaymentCallbackIdempotent(t *testing.T) {
	// Already placed → 200, no cart clear needed.
	store := &fakeStore{byIDOrder: order.Order{OrderID: "EFF-000001", Status: order.StatusPlaced}}
	h, token := newOrderAPI(t, store, &fakeLiner{}, &fakeAddr{addr: defaultAddr}, newFakeStock(), &fakeGateway{})
	rec := doOrder(t, h, http.MethodPost, callbackPath, token, validCallbackBody())
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (idempotent)", rec.Code)
	}
}

func TestPaymentCallbackBadMethod(t *testing.T) {
	h, token := newOrderAPI(t, &fakeStore{}, &fakeLiner{}, &fakeAddr{addr: defaultAddr}, newFakeStock(), &fakeGateway{})
	rec := doOrder(t, h, http.MethodGet, callbackPath, token, "")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

/* ================================================================ default error branches */

func TestOpenCheckoutInternalError(t *testing.T) {
	// A non-ErrAddressNotFound address error reaches the handler's default
	// switch arm → 500.
	lines := []cart.Line{goodAPILine()}
	h, token := newOrderAPI(t,
		&fakeStore{},
		&fakeLiner{lines: lines},
		&fakeAddr{err: errors.New("db connection lost")},
		newFakeStock(),
		&fakeGateway{},
	)
	rec := doOrder(t, h, http.MethodPost, "/api/v1/orders", token, "")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if c := errCode(rec); c != "INTERNAL" {
		t.Errorf("code = %q, want INTERNAL_ERROR", c)
	}
}

func TestPaymentCallbackMalformedJSON(t *testing.T) {
	// Malformed (non-EOF) JSON hits the non-EOF error path → 400.
	h, token := newOrderAPI(t, pendingAPIStore(), &fakeLiner{}, &fakeAddr{addr: defaultAddr}, newFakeStock(), &fakeGateway{})
	rec := doOrder(t, h, http.MethodPost, callbackPath, token, "{invalid json")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (malformed JSON)", rec.Code)
	}
}

func TestPaymentCallbackInternalError(t *testing.T) {
	// MarkPlaced returning an unexpected error hits handleCallbackErr's default → 500.
	store := pendingAPIStore()
	store.markPlacedErr = errors.New("mongo timeout")
	h, token := newOrderAPI(t, store, &fakeLiner{}, &fakeAddr{addr: defaultAddr}, newFakeStock(), &fakeGateway{})
	rec := doOrder(t, h, http.MethodPost, callbackPath, token, validCallbackBody())
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if c := errCode(rec); c != "INTERNAL" {
		t.Errorf("code = %q, want INTERNAL_ERROR", c)
	}
}

/* ================================================================ webhook handler */

const webhookHandlerPath = "/webhooks/razorpay"
const captureJSON = `{"event":"payment.captured","payload":{"payment":{"entity":{"id":"pay_xxx","order_id":"order_rzp","amount":49900,"method":"upi"}}}}`

// newWebhookAPI creates a test server for webhook handler tests with injectable
// store, events, and gateway — no auth token is needed.
func newWebhookAPI(t *testing.T, store order.Store, events order.PaymentEventStore, gw razorpay.Gateway) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	issuer := auth.NewTokenIssuer(orderSecret, auth.TokenTTL)
	svc := order.NewService(order.ServiceConfig{
		Repo:          store,
		Cart:          &fakeLiner{},
		Auth:          &fakeAddr{},
		Catalogue:     newFakeStock(),
		Gateway:       gw,
		Events:        events,
		RazorpayKeyID: "rzp_test_KEY",
		Logger:        logger,
		Now:           func() time.Time { return fixedNow },
	})
	return server.New(server.Deps{
		Config:  config.Config{},
		Mongo:   stubMongoPinger{},
		Order:   order.NewHandler(svc, realParser{issuer}, logger),
		Logger:  logger,
		Version: "test",
		Started: fixedNow,
	})
}

func doWebhook(t *testing.T, h http.Handler, body, sig, eventID string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(http.MethodPost, webhookHandlerPath, reader)
	req.Header.Set("Content-Type", "application/json")
	if sig != "" {
		req.Header.Set("X-Razorpay-Signature", sig)
	}
	if eventID != "" {
		req.Header.Set("X-Razorpay-Event-Id", eventID)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestWebhookMissingSignature(t *testing.T) {
	h := newWebhookAPI(t, &fakeStore{}, &fakeEventStore{}, &fakeGateway{})
	rec := doWebhook(t, h, captureJSON, "", "evt_001")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (missing signature)", rec.Code)
	}
	if c := errCode(rec); c != "BAD_REQUEST" {
		t.Errorf("code = %q, want BAD_REQUEST", c)
	}
}

func TestWebhookInvalidSignature(t *testing.T) {
	gw := &fakeGateway{verifyWHErr: errors.New("hmac mismatch")}
	h := newWebhookAPI(t, &fakeStore{}, &fakeEventStore{}, gw)
	rec := doWebhook(t, h, captureJSON, "bad_sig", "evt_001")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (invalid signature)", rec.Code)
	}
	if c := errCode(rec); c != "BAD_REQUEST" {
		t.Errorf("code = %q, want BAD_REQUEST", c)
	}
}

func TestWebhookMissingEventID(t *testing.T) {
	h := newWebhookAPI(t, &fakeStore{}, &fakeEventStore{}, &fakeGateway{})
	rec := doWebhook(t, h, captureJSON, "valid_sig", "")
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (missing event ID acknowledged without processing)", rec.Code)
	}
}

func TestWebhookUnknownEvent(t *testing.T) {
	h := newWebhookAPI(t, &fakeStore{}, &fakeEventStore{}, &fakeGateway{})
	rec := doWebhook(t, h, `{"event":"subscription.charged","payload":{}}`, "valid_sig", "evt_001")
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (unknown events are acknowledged)", rec.Code)
	}
	var body struct {
		Data map[string]bool `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not JSON: %v (%s)", err, rec.Body.String())
	}
	if !body.Data["received"] {
		t.Error("data.received must be true")
	}
}

func TestWebhookSuccess(t *testing.T) {
	o := order.Order{
		OrderID: "EFF-000001",
		UserID:  testUser,
		Status:  order.StatusPendingPayment,
		Totals:  order.Totals{Total: 49900},
	}
	store := &fakeStore{byRzpOrder: o, markModified: true}
	h := newWebhookAPI(t, store, &fakeEventStore{}, &fakeGateway{})
	rec := doWebhook(t, h, captureJSON, "valid_sig", "evt_001")
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
}

func TestWebhookDuplicateEvent(t *testing.T) {
	events := &fakeEventStore{
		insertErr: order.ErrDuplicateEvent,
		getEvent:  order.PaymentEvent{Processed: true},
	}
	h := newWebhookAPI(t, &fakeStore{}, events, &fakeGateway{})
	rec := doWebhook(t, h, captureJSON, "valid_sig", "evt_001")
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (dup event Processed:true is acknowledged)", rec.Code)
	}
}

func TestWebhookInternalError(t *testing.T) {
	events := &fakeEventStore{insertErr: errors.New("mongo timeout")}
	h := newWebhookAPI(t, &fakeStore{}, events, &fakeGateway{})
	rec := doWebhook(t, h, captureJSON, "valid_sig", "evt_001")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (DB error triggers Razorpay retry)", rec.Code)
	}
	if c := errCode(rec); c != "INTERNAL" {
		t.Errorf("code = %q, want INTERNAL", c)
	}
}

func TestWebhookRequiresPost(t *testing.T) {
	h := newWebhookAPI(t, &fakeStore{}, &fakeEventStore{}, &fakeGateway{})
	rec := doOrder(t, h, http.MethodGet, webhookHandlerPath, "", "")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestPaymentCallbackIdempotentWithMethod(t *testing.T) {
	// An already-placed order with payment method details set exercises the
	// optStr non-nil return path (method, label, vpa are all non-empty strings).
	store := &fakeStore{byIDOrder: order.Order{
		OrderID: "EFF-000001",
		Status:  order.StatusPlaced,
		Payment: order.Payment{
			Method: order.PaymentUPI,
			Label:  "UPI",
			VPA:    "user@okaxis",
		},
	}}
	h, token := newOrderAPI(t, store, &fakeLiner{}, &fakeAddr{addr: defaultAddr}, newFakeStock(), &fakeGateway{})
	rec := doOrder(t, h, http.MethodPost, callbackPath, token, validCallbackBody())
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (idempotent)", rec.Code)
	}
	var body struct {
		Data struct {
			Order struct {
				Payment struct {
					Method *string `json:"method"`
					Label  *string `json:"label"`
					VPA    *string `json:"vpa"`
				} `json:"payment"`
			} `json:"order"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	p := body.Data.Order.Payment
	if p.Method == nil || *p.Method != "upi" {
		t.Errorf("method = %v, want non-nil \"upi\"", p.Method)
	}
	if p.Label == nil || *p.Label != "UPI" {
		t.Errorf("label = %v, want non-nil \"UPI\"", p.Label)
	}
	if p.VPA == nil || *p.VPA != "user@okaxis" {
		t.Errorf("vpa = %v, want non-nil \"user@okaxis\"", p.VPA)
	}
}

/* ======================================================== GET /orders */

func newListAPI(t *testing.T, store order.Store) (http.Handler, string) {
	t.Helper()
	return newOrderAPI(t, store, &fakeLiner{}, &fakeAddr{addr: defaultAddr}, newFakeStock(), &fakeGateway{})
}

func TestListOrders_NoOrdersReturnsEmptyArray(t *testing.T) {
	// No orders is 200 with [] — never 404. The JSON must be an empty array,
	// not null, so the frontend can always iterate without a nil-check.
	h, token := newListAPI(t, &fakeStore{})
	rec := doOrder(t, h, http.MethodGet, "/api/v1/orders", token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Data struct {
			Orders []json.RawMessage `json:"orders"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("bad JSON: %v (%s)", err, rec.Body.String())
	}
	if body.Data.Orders == nil {
		t.Error("orders must be [] not null for an empty result")
	}
	if len(body.Data.Orders) != 0 {
		t.Errorf("orders len = %d, want 0", len(body.Data.Orders))
	}
}

func TestListOrders_ReturnsOrdersInRepoOrder(t *testing.T) {
	// The handler passes through whatever the repository returns. The repository
	// sorts by createdAt desc; the test verifies orderId ordering end-to-end.
	now := time.Now()
	store := &fakeStore{listOrders: []order.Order{
		{OrderID: "EFF-000002", Status: order.StatusPlaced, CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
		{OrderID: "EFF-000001", Status: order.StatusPendingPayment, CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour)},
	}}
	h, token := newListAPI(t, store)
	rec := doOrder(t, h, http.MethodGet, "/api/v1/orders", token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var body struct {
		Data struct {
			Orders []struct {
				OrderID string `json:"orderId"`
				Status  string `json:"status"`
			} `json:"orders"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if len(body.Data.Orders) != 2 {
		t.Fatalf("orders len = %d, want 2", len(body.Data.Orders))
	}
	if body.Data.Orders[0].OrderID != "EFF-000002" {
		t.Errorf("orders[0].orderId = %q, want EFF-000002 (newest first)", body.Data.Orders[0].OrderID)
	}
	if body.Data.Orders[1].OrderID != "EFF-000001" {
		t.Errorf("orders[1].orderId = %q, want EFF-000001", body.Data.Orders[1].OrderID)
	}
}

func TestListOrders_UnauthenticatedReturns401(t *testing.T) {
	h, _ := newListAPI(t, &fakeStore{})
	rec := doOrder(t, h, http.MethodGet, "/api/v1/orders", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestListOrders_RepoErrorReturns500(t *testing.T) {
	store := &fakeStore{listErr: errors.New("db down")}
	h, token := newListAPI(t, store)
	rec := doOrder(t, h, http.MethodGet, "/api/v1/orders", token, "")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

/* ======================================================== GET /orders/{orderId} */

func newGetAPI(t *testing.T, store order.Store) (http.Handler, string) {
	t.Helper()
	return newOrderAPI(t, store, &fakeLiner{}, &fakeAddr{addr: defaultAddr}, newFakeStock(), &fakeGateway{})
}

func TestGetOrder_UnauthenticatedReturns401(t *testing.T) {
	h, _ := newGetAPI(t, &fakeStore{})
	rec := doOrder(t, h, http.MethodGet, "/api/v1/orders/EFF-000001", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestGetOrder_NotFoundReturns404(t *testing.T) {
	store := &fakeStore{byIDErr: order.ErrOrderNotFound}
	h, token := newGetAPI(t, store)
	rec := doOrder(t, h, http.MethodGet, "/api/v1/orders/EFF-000001", token, "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestGetOrder_OtherUserResponseByteIdenticalToNotFound(t *testing.T) {
	// ByOrderID scopes by both orderId AND userId in a single query, so it
	// returns ErrOrderNotFound for both a non-existent order and one that
	// belongs to a different shopper. The handler must respond identically in
	// both cases so the endpoint cannot be used to enumerate real order IDs.
	store := &fakeStore{byIDErr: order.ErrOrderNotFound}
	h, token := newGetAPI(t, store)

	// Simulate "order does not exist"
	rec1 := doOrder(t, h, http.MethodGet, "/api/v1/orders/EFF-000001", token, "")
	// Simulate "order belongs to another user" — same sentinel, different ID
	rec2 := doOrder(t, h, http.MethodGet, "/api/v1/orders/EFF-000002", token, "")

	if rec1.Code != http.StatusNotFound || rec2.Code != http.StatusNotFound {
		t.Errorf("status = %d and %d, want both 404", rec1.Code, rec2.Code)
	}
	if rec1.Body.String() != rec2.Body.String() {
		t.Errorf("response bodies differ:\n  not-found:   %s\n  other-user:  %s",
			rec1.Body.String(), rec2.Body.String())
	}
}

func TestGetOrder_PendingOrderOmitsPlacedAtPaymentAndEta(t *testing.T) {
	// A pending_payment order has no placedAt, no payment method and no ETA.
	// These must be absent (omitempty on nil pointer), never a zero time or
	// empty string (roadmap.md §GET /api/v1/orders/{orderId}).
	now := time.Now()
	store := &fakeStore{byIDOrder: order.Order{
		OrderID:   "EFF-000001",
		Status:    order.StatusPendingPayment,
		CreatedAt: now,
		ExpiresAt: now.Add(15 * time.Minute),
	}}
	h, token := newGetAPI(t, store)
	rec := doOrder(t, h, http.MethodGet, "/api/v1/orders/EFF-000001", token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}

	// Use a raw map so we can distinguish absent fields from null ones.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal(raw["data"], &data); err != nil {
		t.Fatalf("bad data JSON: %v", err)
	}
	var o map[string]json.RawMessage
	if err := json.Unmarshal(data["order"], &o); err != nil {
		t.Fatalf("bad order JSON: %v", err)
	}

	if _, ok := o["placedAt"]; ok {
		t.Errorf("placedAt must be absent for a pending order, got %s", o["placedAt"])
	}
	if _, ok := o["payment"]; ok {
		t.Errorf("payment must be absent for a pending order, got %s", o["payment"])
	}
	if _, ok := o["etaText"]; ok {
		t.Errorf("etaText must be absent for a pending order, got %s", o["etaText"])
	}
}

func TestGetOrder_PlacedOrderCarriesPlacedAtEtaAndPayment(t *testing.T) {
	now := time.Now()
	pt := now.Add(-5 * time.Minute)
	method := order.PaymentMethod("upi")
	label := method.Label()
	vpa := "user@okaxis"
	store := &fakeStore{byIDOrder: order.Order{
		OrderID:   "EFF-000001",
		Status:    order.StatusPlaced,
		CreatedAt: now,
		ExpiresAt: now.Add(15 * time.Minute),
		PlacedAt:  &pt,
		Payment: order.Payment{
			Method: method,
			Label:  label,
			VPA:    vpa,
		},
	}}
	h, token := newGetAPI(t, store)
	rec := doOrder(t, h, http.MethodGet, "/api/v1/orders/EFF-000001", token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}

	var body struct {
		Data struct {
			Order struct {
				Status   string `json:"status"`
				PlacedAt string `json:"placedAt"`
				EtaText  string `json:"etaText"`
				Payment  struct {
					Method *string `json:"method"`
					Label  *string `json:"label"`
					VPA    *string `json:"vpa"`
				} `json:"payment"`
			} `json:"order"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	o := body.Data.Order
	if o.Status != "placed" {
		t.Errorf("status = %q, want placed", o.Status)
	}
	if o.PlacedAt == "" {
		t.Error("placedAt must be present on a placed order")
	}
	if o.EtaText == "" {
		t.Error("etaText must be present on a placed order")
	}
	if o.Payment.Method == nil || *o.Payment.Method != "upi" {
		t.Errorf("payment.method = %v, want non-nil upi", o.Payment.Method)
	}
}

func TestGetOrder_RepoErrorReturns500(t *testing.T) {
	store := &fakeStore{byIDErr: errors.New("db down")}
	h, token := newGetAPI(t, store)
	rec := doOrder(t, h, http.MethodGet, "/api/v1/orders/EFF-000001", token, "")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}
