package cart_test

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
	"github.com/enerzia/enerzia-be/internal/server"
)

var cartSecret = []byte("cart-test-secret-at-least-32-characters")

// realParser wires the genuine JWT verifier into the cart handler, so these
// tests exercise the real auth middleware rather than a permissive stub.
type realParser struct{ issuer *auth.TokenIssuer }

func (p realParser) ParseToken(token string) (auth.Claims, error) { return p.issuer.Parse(token) }

type stubPinger struct{}

func (stubPinger) Ping(context.Context) error { return nil }

// newCartAPI wires a store through the real service, handler, auth middleware
// and router, and returns a valid bearer token for testUser.
func newCartAPI(t *testing.T, store cart.Store, products cart.ProductSource) (http.Handler, string) {
	t.Helper()

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	issuer := auth.NewTokenIssuer(cartSecret, auth.TokenTTL)

	token, _, err := issuer.Issue(auth.User{ID: testUser, Phone: "9876543210"})
	if err != nil {
		t.Fatalf("issuing a test token: %v", err)
	}

	handler := server.New(server.Deps{
		Config:  config.Config{},
		Mongo:   stubPinger{},
		Cart:    cart.NewHandler(cart.NewService(store, products), realParser{issuer}, logger),
		Logger:  logger,
		Version: "test",
		Started: time.Now(),
	})
	return handler, token
}

func do(t *testing.T, h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
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

type cartBody struct {
	Data struct {
		Lines []struct {
			ProductID string `json:"productId"`
			Qty       int    `json:"qty"`
			UnitPrice int64  `json:"unitPrice"`
			LineTotal int64  `json:"lineTotal"`
		} `json:"lines"`
		Totals struct {
			MRPTotal int64 `json:"mrpTotal"`
			Subtotal int64 `json:"subtotal"`
			Savings  int64 `json:"savings"`
			Shipping int64 `json:"shipping"`
			Total    int64 `json:"total"`
		} `json:"totals"`
		FreeShipping struct {
			ThresholdAmount int64 `json:"thresholdAmount"`
			Qualified       bool  `json:"qualified"`
			RemainingAmount int64 `json:"remainingAmount"`
		} `json:"freeShipping"`
		ItemCount         int  `json:"itemCount"`
		HasBlockingIssues bool `json:"hasBlockingIssues"`
	} `json:"data"`
}

func decodeCart(t *testing.T, rec *httptest.ResponseRecorder) cartBody {
	t.Helper()
	var body cartBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v (%s)", err, rec.Body.String())
	}
	return body
}

func errCode(t *testing.T, rec *httptest.ResponseRecorder) string {
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

/* ------------------------------------------------------------ auth required */

func TestEveryCartRouteRequiresAuth(t *testing.T) {
	products := newStubProducts()
	h, _ := newCartAPI(t, newMemStore(), products)
	id := "powder-100g"

	tests := []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/cart", ""},
		{http.MethodDelete, "/api/v1/cart", ""},
		{http.MethodPost, "/api/v1/cart/items", `{"productId":"` + id + `"}`},
		{http.MethodPatch, "/api/v1/cart/items/" + id, `{"qty":2}`},
		{http.MethodDelete, "/api/v1/cart/items/" + id, ""},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			rec := do(t, h, tt.method, tt.path, "", tt.body)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if got := errCode(t, rec); got != "UNAUTHORIZED" {
				t.Errorf("error.code = %q, want UNAUTHORIZED", got)
			}
		})
	}
}

func TestCartRejectsAForgedToken(t *testing.T) {
	h, _ := newCartAPI(t, newMemStore(), newStubProducts())

	forged := auth.NewTokenIssuer([]byte("a-totally-different-secret-key-32ch"), auth.TokenTTL)
	token, _, err := forged.Issue(auth.User{ID: bson.NewObjectID(), Phone: "9000000000"})
	if err != nil {
		t.Fatalf("issuing the forged token: %v", err)
	}

	if rec := do(t, h, http.MethodGet, "/api/v1/cart", token, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for a token signed with another key", rec.Code)
	}
}

/* -------------------------------------------------------------- GET /cart */

func TestGetCartIsEmptyNotFound(t *testing.T) {
	h, token := newCartAPI(t, newMemStore(), newStubProducts())

	rec := do(t, h, http.MethodGet, "/api/v1/cart", token, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — an empty cart is not a 404", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"lines":[]`) {
		t.Errorf("body = %s, want lines to serialise as []", rec.Body.String())
	}
	body := decodeCart(t, rec)
	if body.Data.FreeShipping.ThresholdAmount != cart.FreeShippingThreshold {
		t.Errorf("threshold = %d, want %d", body.Data.FreeShipping.ThresholdAmount, cart.FreeShippingThreshold)
	}
	if body.Data.HasBlockingIssues {
		t.Error("an empty cart cannot block checkout")
	}
}

func TestGetCartReturnsPricedLines(t *testing.T) {
	store, products := newMemStore(), newStubProducts()
	id := catalogue.ID("tablets-120")
	store.carts[testUser] = cart.Cart{UserID: testUser, Lines: []cart.StoredLine{{ProductID: id, Qty: 3}}}
	h, token := newCartAPI(t, store, products)

	rec := do(t, h, http.MethodGet, "/api/v1/cart", token, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	body := decodeCart(t, rec)

	if body.Data.ItemCount != 3 {
		t.Errorf("itemCount = %d, want 3", body.Data.ItemCount)
	}
	if body.Data.Lines[0].ProductID != string(id) {
		t.Errorf("variantId = %q, want %q", body.Data.Lines[0].ProductID, id)
	}
	// The numbers the shipped UI showed for this cart.
	if body.Data.Totals.MRPTotal != 141000 || body.Data.Totals.Subtotal != 114000 {
		t.Errorf("totals = %+v", body.Data.Totals)
	}
	if body.Data.Totals.Savings != 27000 || body.Data.Totals.Shipping != 0 {
		t.Errorf("savings/shipping = %d/%d", body.Data.Totals.Savings, body.Data.Totals.Shipping)
	}
	if !body.Data.FreeShipping.Qualified {
		t.Error("a ₹1,140 cart should qualify for free delivery")
	}
}

/* -------------------------------------------------------- POST /cart/items */

func TestAddItemEndpoint(t *testing.T) {
	products := newStubProducts()
	h, token := newCartAPI(t, newMemStore(), products)
	id := catalogue.ID("tablets-120")

	rec := do(t, h, http.MethodPost, "/api/v1/cart/items", token,
		`{"productId":"`+string(id)+`","qty":1}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	body := decodeCart(t, rec)

	if len(body.Data.Lines) != 1 || body.Data.Lines[0].ProductID != string(id) {
		t.Fatalf("lines = %+v", body.Data.Lines)
	}
	// Below ₹499, so delivery is charged and the nudge shows the shortfall.
	if body.Data.Totals.Shipping != cart.ShippingFee {
		t.Errorf("shipping = %d, want %d", body.Data.Totals.Shipping, cart.ShippingFee)
	}
	if body.Data.FreeShipping.RemainingAmount != 11900 {
		t.Errorf("remaining = %d, want 11900 (₹119)", body.Data.FreeShipping.RemainingAmount)
	}
}

func TestAddItemDefaultsQuantityToOne(t *testing.T) {
	products := newStubProducts()
	h, token := newCartAPI(t, newMemStore(), products)

	rec := do(t, h, http.MethodPost, "/api/v1/cart/items", token,
		`{"productId":"powder-100g"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if got := decodeCart(t, rec).Data.Lines[0].Qty; got != 1 {
		t.Errorf("qty = %d, want the default of 1", got)
	}
}

func TestAddItemRejectsAnExplicitZero(t *testing.T) {
	// Omitted means "one"; an explicit 0 is a mistake, not a default.
	products := newStubProducts()
	h, token := newCartAPI(t, newMemStore(), products)

	rec := do(t, h, http.MethodPost, "/api/v1/cart/items", token,
		`{"productId":"powder-100g","qty":0}`)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
}

func TestAddItemEndpointRejectsAnUnknownProduct(t *testing.T) {
	h, token := newCartAPI(t, newMemStore(), newStubProducts())

	rec := do(t, h, http.MethodPost, "/api/v1/cart/items", token,
		`{"productId":"ghost-1kg"}`)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if got := errCode(t, rec); got != "VALIDATION_ERROR" {
		t.Errorf("error.code = %q, want VALIDATION_ERROR", got)
	}
}

func TestAddItemRejectsAnEmptyProductID(t *testing.T) {
	// An empty id names nothing the shop sells.
	h, token := newCartAPI(t, newMemStore(), newStubProducts())

	rec := do(t, h, http.MethodPost, "/api/v1/cart/items", token, `{"productId":""}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", rec.Code)
	}
}

func TestAddItemEndpointRejectsASoldOutProduct(t *testing.T) {
	products := newStubProducts()
	products.setStock(t, "powder-100g", 0)
	h, token := newCartAPI(t, newMemStore(), products)

	rec := do(t, h, http.MethodPost, "/api/v1/cart/items", token,
		`{"productId":"powder-100g"}`)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 (%s)", rec.Code, rec.Body.String())
	}
}

func TestAddItemIgnoresAClientSuppliedPrice(t *testing.T) {
	// Prices are server-authoritative; an unknown field is rejected outright.
	products := newStubProducts()
	h, token := newCartAPI(t, newMemStore(), products)

	rec := do(t, h, http.MethodPost, "/api/v1/cart/items", token,
		`{"productId":"powder-100g","qty":1,"unitPrice":1}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for an unknown field", rec.Code)
	}
}

func TestAddItemRejectsMalformedBodies(t *testing.T) {
	h, token := newCartAPI(t, newMemStore(), newStubProducts())

	for _, body := range []string{`nope`, `{"productId":`, ``} {
		rec := do(t, h, http.MethodPost, "/api/v1/cart/items", token, body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %q → status %d, want 400", body, rec.Code)
		}
	}
}

func TestAddItemHidesInternalFailures(t *testing.T) {
	products := newStubProducts()
	id := catalogue.ID("powder-100g")
	products.err = errors.New("cluster0-shard-00.mongodb.net refused")
	h, token := newCartAPI(t, newMemStore(), products)

	rec := do(t, h, http.MethodPost, "/api/v1/cart/items", token, `{"productId":"`+string(id)+`"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "mongodb.net") {
		t.Errorf("response leaks infrastructure detail: %s", rec.Body.String())
	}
}

/* ---------------------------------------- PATCH /cart/items/{variantId} */

func TestSetQtyEndpoint(t *testing.T) {
	products := newStubProducts()
	h, token := newCartAPI(t, newMemStore(), products)
	id := "tablets-120"
	do(t, h, http.MethodPost, "/api/v1/cart/items", token, `{"productId":"`+id+`","qty":1}`)

	rec := do(t, h, http.MethodPatch, "/api/v1/cart/items/"+id, token, `{"qty":3}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if got := decodeCart(t, rec).Data.Lines[0].Qty; got != 3 {
		t.Errorf("qty = %d, want 3", got)
	}
}

func TestSetQtyZeroRemovesViaTheEndpoint(t *testing.T) {
	products := newStubProducts()
	h, token := newCartAPI(t, newMemStore(), products)
	id := "tablets-120"
	do(t, h, http.MethodPost, "/api/v1/cart/items", token, `{"productId":"`+id+`","qty":1}`)

	rec := do(t, h, http.MethodPatch, "/api/v1/cart/items/"+id, token, `{"qty":0}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if n := len(decodeCart(t, rec).Data.Lines); n != 0 {
		t.Errorf("%d lines, want the line removed", n)
	}
}

func TestSetQtyRequiresTheField(t *testing.T) {
	// An empty body must not be read as "set it to zero" and silently delete.
	products := newStubProducts()
	h, token := newCartAPI(t, newMemStore(), products)
	id := "tablets-120"
	do(t, h, http.MethodPost, "/api/v1/cart/items", token, `{"productId":"`+id+`","qty":1}`)

	rec := do(t, h, http.MethodPatch, "/api/v1/cart/items/"+id, token, `{}`)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
}

func TestSetQtyOnAnUnknownLineIs404(t *testing.T) {
	h, token := newCartAPI(t, newMemStore(), newStubProducts())

	rec := do(t, h, http.MethodPatch, "/api/v1/cart/items/ghost-1kg", token, `{"qty":2}`)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if got := errCode(t, rec); got != "NOT_FOUND" {
		t.Errorf("error.code = %q, want NOT_FOUND", got)
	}
}

func TestSetQtyRejectsAMalformedBody(t *testing.T) {
	h, token := newCartAPI(t, newMemStore(), newStubProducts())

	rec := do(t, h, http.MethodPatch, "/api/v1/cart/items/ghost-1kg", token, `{`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestSetQtyRejectsOutOfRange(t *testing.T) {
	products := newStubProducts()
	h, token := newCartAPI(t, newMemStore(), products)
	id := "powder-100g"
	do(t, h, http.MethodPost, "/api/v1/cart/items", token, `{"productId":"`+id+`","qty":1}`)

	rec := do(t, h, http.MethodPatch, "/api/v1/cart/items/"+id, token, `{"qty":500}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", rec.Code)
	}
}

/* --------------------------------------- DELETE /cart/items/{variantId} */

func TestRemoveItemEndpoint(t *testing.T) {
	products := newStubProducts()
	h, token := newCartAPI(t, newMemStore(), products)
	id := "powder-100g"
	do(t, h, http.MethodPost, "/api/v1/cart/items", token, `{"productId":"`+id+`","qty":2}`)

	rec := do(t, h, http.MethodDelete, "/api/v1/cart/items/"+id, token, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if n := len(decodeCart(t, rec).Data.Lines); n != 0 {
		t.Errorf("%d lines, want 0", n)
	}
}

func TestRemoveUnknownItemIs404(t *testing.T) {
	h, token := newCartAPI(t, newMemStore(), newStubProducts())

	rec := do(t, h, http.MethodDelete, "/api/v1/cart/items/ghost-1kg", token, "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

/* ----------------------------------------------------- DELETE /cart */

func TestClearCartEndpoint(t *testing.T) {
	products := newStubProducts()
	h, token := newCartAPI(t, newMemStore(), products)
	do(t, h, http.MethodPost, "/api/v1/cart/items", token,
		`{"variantId":"`+"powder-100g"+`","qty":2}`)
	do(t, h, http.MethodPost, "/api/v1/cart/items", token,
		`{"variantId":"`+"tablets-120"+`","qty":1}`)

	rec := do(t, h, http.MethodDelete, "/api/v1/cart", token, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	body := decodeCart(t, rec)
	if len(body.Data.Lines) != 0 || body.Data.ItemCount != 0 {
		t.Errorf("cart not emptied: %+v", body.Data)
	}
}

/* ---------------------------------------------------------- routing */

func TestCartRoutesRejectWrongMethods(t *testing.T) {
	products := newStubProducts()
	h, token := newCartAPI(t, newMemStore(), products)
	id := "powder-100g"

	tests := []struct{ method, path string }{
		{http.MethodPut, "/api/v1/cart"},
		{http.MethodGet, "/api/v1/cart/items"},
		{http.MethodPost, "/api/v1/cart/items/" + id},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			rec := do(t, h, tt.method, tt.path, token, "")
			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("status = %d, want 405", rec.Code)
			}
		})
	}
}

func TestCartResponsesAreJSON(t *testing.T) {
	h, token := newCartAPI(t, newMemStore(), newStubProducts())

	rec := do(t, h, http.MethodGet, "/api/v1/cart", token, "")
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
}

/* -------------------------------------------- end-to-end through HTTP */

func TestFullCartJourneyOverHTTP(t *testing.T) {
	products := newStubProducts()
	h, token := newCartAPI(t, newMemStore(), products)
	tablets := "tablets-120"
	powder := "powder-100g"

	do(t, h, http.MethodPost, "/api/v1/cart/items", token, `{"productId":"`+tablets+`","qty":1}`)

	rec := do(t, h, http.MethodPatch, "/api/v1/cart/items/"+tablets, token, `{"qty":2}`)
	body := decodeCart(t, rec)
	if body.Data.Totals.Subtotal != 76000 {
		t.Fatalf("subtotal = %d, want 76000", body.Data.Totals.Subtotal)
	}
	if !body.Data.FreeShipping.Qualified || body.Data.Totals.Shipping != 0 {
		t.Errorf("₹760 should clear the ₹499 threshold: %+v", body.Data)
	}

	rec = do(t, h, http.MethodPost, "/api/v1/cart/items", token, `{"productId":"`+powder+`","qty":1}`)
	if n := len(decodeCart(t, rec).Data.Lines); n != 2 {
		t.Fatalf("%d lines, want 2", n)
	}

	rec = do(t, h, http.MethodDelete, "/api/v1/cart/items/"+powder, token, "")
	if n := len(decodeCart(t, rec).Data.Lines); n != 1 {
		t.Fatalf("%d lines after remove, want 1", n)
	}

	rec = do(t, h, http.MethodDelete, "/api/v1/cart", token, "")
	if decodeCart(t, rec).Data.ItemCount != 0 {
		t.Error("cart not empty after clear")
	}

	rec = do(t, h, http.MethodGet, "/api/v1/cart", token, "")
	if decodeCart(t, rec).Data.ItemCount != 0 {
		t.Error("the cleared cart came back non-empty on re-read")
	}
}
