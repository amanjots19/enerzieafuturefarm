package order_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/enerzia/enerzia-be/internal/admin"
	"github.com/enerzia/enerzia-be/internal/auth"
	"github.com/enerzia/enerzia-be/internal/catalogue"
	"github.com/enerzia/enerzia-be/internal/config"
	"github.com/enerzia/enerzia-be/internal/order"
	"github.com/enerzia/enerzia-be/internal/server"
)

var adminOrderSecret = []byte("admin-order-secret-at-least-32-bytes-long")

var errAdminBoom = errors.New("db down")

// testOrigin is a fully configured shipping origin. Tests that need the
// unconfigured case build their own handler with a zero value.
var testOrigin = config.ShipFromAddress{
	Name:  "Enerzeia Future Farm",
	Line1: "Plot 14, Sector 58",
	City:  "Faridabad",
	State: "Haryana",
	Pin:   "121004",
	Phone: "9812345678",
}

type adminTokenParser struct{ issuer *admin.TokenIssuer }

func (p adminTokenParser) ParseToken(tok string) (admin.Claims, error) {
	return p.issuer.Parse(tok)
}

// stubAdminStore records the filter it was handed so the tests can assert what
// the handler asked for, not merely what came back.
type stubAdminStore struct {
	got    order.AdminFilter
	calls  int
	orders []order.Order
	err    error

	// one is returned by ByOrderIDUnscoped; oneErr overrides it.
	one     order.Order
	oneErr  error
	gotID   string
	idCalls int

	setCalls       []setFulfilmentCall
	setErr         error
	setNotModified bool
}

// setCalls records every guarded write attempt, so tests can assert the FROM
// state the handler passed — that argument is the concurrency guard.
type setFulfilmentCall struct {
	orderID  string
	from, to order.Fulfilment
}

func (s *stubAdminStore) SetFulfilment(
	_ context.Context, id string, from, to order.Fulfilment, _ time.Time,
) (bool, error) {
	s.setCalls = append(s.setCalls, setFulfilmentCall{id, from, to})
	if s.setErr != nil {
		return false, s.setErr
	}
	return !s.setNotModified, nil
}

func (s *stubAdminStore) ByOrderIDUnscoped(_ context.Context, id string) (order.Order, error) {
	s.gotID = id
	s.idCalls++
	if s.oneErr != nil {
		return order.Order{}, s.oneErr
	}
	return s.one, nil
}

func (s *stubAdminStore) ListAll(_ context.Context, f order.AdminFilter) ([]order.Order, error) {
	s.got = f
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	if s.orders == nil {
		return []order.Order{}, nil
	}
	return s.orders, nil
}

// newAdminOrderAPI wires the stub store through the real handler and router, so
// routing, the admin middleware and encoding are all exercised.
func newAdminOrderAPI(t *testing.T, store *stubAdminStore) (http.Handler, string) {
	t.Helper()
	// Tests default to production so the paid-only default is what most of them
	// exercise; newAdminOrderAPIIn covers the other environments.
	return newAdminOrderAPIIn(t, store, config.EnvProduction)
}

func newAdminOrderAPIIn(
	t *testing.T,
	store *stubAdminStore,
	env config.Environment,
) (http.Handler, string) {
	t.Helper()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	issuer := admin.NewTokenIssuer(adminOrderSecret, admin.TokenTTL)
	h := server.New(server.Deps{
		Config:     config.Config{},
		Mongo:      stubAdminPinger{},
		AdminOrder: order.NewAdminHandler(store, adminTokenParser{issuer}, logger, env, testOrigin),
		Logger:     logger,
		Version:    "test",
		Started:    time.Now(),
	})
	tok, _, err := issuer.Issue("ops@enerzia.in")
	if err != nil {
		t.Fatalf("issue admin token: %v", err)
	}
	return h, tok
}

type stubAdminPinger struct{}

func (stubAdminPinger) Ping(context.Context) error { return nil }

func getAdmin(t *testing.T, h http.Handler, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// adminOrderFixture is a placed order with everything an operator would look at.
func adminOrderFixture(id string, created time.Time) order.Order {
	placed := created.Add(time.Minute)
	captured := placed
	return order.Order{
		OrderID: id,
		UserID:  bson.NewObjectID(),
		Status:  order.StatusPlaced,
		Lines: []order.Line{{
			ProductID: "powder-100g", Name: "Pure Spirulina Powder - 100 g",
			Form: catalogue.FormPowder, UnitPrice: 20000, UnitMrp: 25000,
			Qty: 1, LineTotal: 20000,
		}},
		Totals: order.Totals{
			MRPTotal: 25000, Subtotal: 20000, Savings: 5000, Shipping: 5000, Total: 25000,
		},
		Payment: order.Payment{
			Provider: "razorpay", Status: order.PaymentStatusCaptured,
			Amount: 25000, Currency: "INR",
			RazorpayOrderID: "order_Pk9", RazorpayPaymentID: "pay_Pk9",
			RazorpaySignature: "9f2b3c-secret-hmac",
			Method:            order.PaymentUPI, Label: "UPI", VPA: "ananya@okaxis",
			CapturedAt: &captured,
		},
		ShippingAddress: auth.Address{
			Name: "Ananya Sharma", Email: "a@b.co", Phone: "9811111111",
			Line1: "12, Anand Residency", City: "Pune", State: "Maharashtra", Pin: "411001",
		},
		CustomerPhone: "9811111111",
		CreatedAt:     created,
		ExpiresAt:     created.Add(15 * time.Minute),
		PlacedAt:      &placed,
		UpdatedAt:     placed,
	}
}

func decodeAdminList(t *testing.T, rec *httptest.ResponseRecorder) struct {
	Data struct {
		Orders     []map[string]any `json:"orders"`
		NextBefore *time.Time       `json:"nextBefore"`
		Count      int              `json:"count"`
	} `json:"data"`
} {
	t.Helper()
	var body struct {
		Data struct {
			Orders     []map[string]any `json:"orders"`
			NextBefore *time.Time       `json:"nextBefore"`
			Count      int              `json:"count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding %s: %v", rec.Body.String(), err)
	}
	return body
}

/* ------------------------------------------------------------------- auth */

func TestAdminOrdersRequiresAnAdminToken(t *testing.T) {
	store := &stubAdminStore{}
	h, _ := newAdminOrderAPI(t, store)

	tests := []struct {
		name  string
		token string
	}{
		{"no token", ""},
		{"garbage token", "not-a-jwt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := getAdmin(t, h, "/api/v1/admin/orders", tt.token)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401 (%s)", rec.Code, rec.Body.String())
			}
		})
	}
	// The store must never be reached by an unauthenticated caller — a 401 that
	// still ran the query would leak timing and load.
	if store.calls != 0 {
		t.Errorf("store was called %d times behind a 401", store.calls)
	}
}

func TestAdminOrdersRejectsAShopperToken(t *testing.T) {
	// A shopper token is signed with the same secret but a different issuer.
	// Accepting one would hand every customer the whole order book.
	store := &stubAdminStore{}
	h, _ := newAdminOrderAPI(t, store)
	shopper := auth.NewTokenIssuer(adminOrderSecret, auth.TokenTTL)
	tok, _, err := shopper.Issue(auth.User{ID: bson.NewObjectID(), Phone: "9876543210"})
	if err != nil {
		t.Fatalf("issue shopper token: %v", err)
	}

	rec := getAdmin(t, h, "/api/v1/admin/orders", tok)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 (%s)", rec.Code, rec.Body.String())
	}
	if store.calls != 0 {
		t.Errorf("store was called %d times for a shopper token", store.calls)
	}
}

/* ---------------------------------------------------------------- filters */

func TestAdminOrdersDefaultsToTheWorkQueueInProduction(t *testing.T) {
	store := &stubAdminStore{}
	h, tok := newAdminOrderAPI(t, store)

	rec := getAdmin(t, h, "/api/v1/admin/orders", tok)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	// Omitting ?status must not mean "everything": an abandoned checkout is not
	// an order, and listing them buries the real work.
	want := order.PaidStatuses()
	if len(store.got.Statuses) != len(want) {
		t.Fatalf("statuses = %v, want the %d paid statuses", store.got.Statuses, len(want))
	}
	for i, s := range want {
		if store.got.Statuses[i] != s {
			t.Errorf("statuses[%d] = %q, want %q", i, store.got.Statuses[i], s)
		}
	}
	if store.got.Limit != order.AdminListDefaultLimit {
		t.Errorf("limit = %d, want %d", store.got.Limit, order.AdminListDefaultLimit)
	}
	if store.got.Before != nil {
		t.Errorf("before = %v, want nil", store.got.Before)
	}
	if store.got.Fulfilments != nil {
		t.Errorf("fulfilments = %v, want nil (no clause)", store.got.Fulfilments)
	}
	// An empty book is [] and a 200, not a 404 and not null.
	if !strings.Contains(rec.Body.String(), `"orders":[]`) {
		t.Errorf("body = %s, want an empty array", rec.Body.String())
	}
}

func TestAdminOrdersStatusAllLiftsTheFilter(t *testing.T) {
	store := &stubAdminStore{}
	h, tok := newAdminOrderAPI(t, store)

	rec := getAdmin(t, h, "/api/v1/admin/orders?status=all", tok)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	// nil is "no clause", which is what support needs to find a failed payment.
	if store.got.Statuses != nil {
		t.Errorf("statuses = %v, want nil for ?status=all", store.got.Statuses)
	}
}

func TestAdminOrdersParsesFilters(t *testing.T) {
	before := time.Date(2026, 8, 14, 9, 12, 4, 0, time.UTC)

	tests := []struct {
		name            string
		query           string
		wantStatuses    []order.Status
		wantFulfilments []order.Fulfilment
		wantLimit       int
		wantBefore      *time.Time
	}{
		{
			name:         "a single status",
			query:        "?status=placed",
			wantStatuses: []order.Status{order.StatusPlaced},
			wantLimit:    order.AdminListDefaultLimit,
		},
		{
			name:         "a comma-separated set",
			query:        "?status=placed,cancelled",
			wantStatuses: []order.Status{order.StatusPlaced, order.StatusCancelled},
			wantLimit:    order.AdminListDefaultLimit,
		},
		{
			name:         "blank entries and stray commas are ignored",
			query:        "?status=placed,,cancelled,",
			wantStatuses: []order.Status{order.StatusPlaced, order.StatusCancelled},
			wantLimit:    order.AdminListDefaultLimit,
		},
		{
			name:            "fulfilment none is the day-to-day queue",
			query:           "?fulfilment=none",
			wantStatuses:    order.PaidStatuses(),
			wantFulfilments: []order.Fulfilment{order.FulfilmentNone},
			wantLimit:       order.AdminListDefaultLimit,
		},
		{
			name:            "fulfilment set",
			query:           "?fulfilment=none,packed,in_transit",
			wantStatuses:    order.PaidStatuses(),
			wantFulfilments: []order.Fulfilment{order.FulfilmentNone, order.FulfilmentPacked, order.FulfilmentInTransit},
			wantLimit:       order.AdminListDefaultLimit,
		},
		{
			name:         "limit and cursor",
			query:        "?limit=200&before=2026-08-14T09:12:04Z",
			wantStatuses: order.PaidStatuses(),
			wantLimit:    200,
			wantBefore:   &before,
		},
		{
			name:         "limit of one is allowed",
			query:        "?limit=1",
			wantStatuses: order.PaidStatuses(),
			wantLimit:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &stubAdminStore{}
			h, tok := newAdminOrderAPI(t, store)

			rec := getAdmin(t, h, "/api/v1/admin/orders"+tt.query, tok)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
			}

			if !sameStatuses(store.got.Statuses, tt.wantStatuses) {
				t.Errorf("statuses = %v, want %v", store.got.Statuses, tt.wantStatuses)
			}
			if !sameFulfilments(store.got.Fulfilments, tt.wantFulfilments) {
				t.Errorf("fulfilments = %v, want %v", store.got.Fulfilments, tt.wantFulfilments)
			}
			if store.got.Limit != tt.wantLimit {
				t.Errorf("limit = %d, want %d", store.got.Limit, tt.wantLimit)
			}
			switch {
			case tt.wantBefore == nil && store.got.Before != nil:
				t.Errorf("before = %v, want nil", store.got.Before)
			case tt.wantBefore != nil && store.got.Before == nil:
				t.Errorf("before = nil, want %v", tt.wantBefore)
			case tt.wantBefore != nil && !store.got.Before.Equal(*tt.wantBefore):
				t.Errorf("before = %v, want %v", store.got.Before, tt.wantBefore)
			}
		})
	}
}

func TestAdminOrdersRejectsBadFilters(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"unknown status", "?status=nonsense"},
		{"fulfilment value used as a status", "?status=in_transit"},
		{"status is only commas", "?status=,,"},
		{"unknown fulfilment", "?fulfilment=nonsense"},
		{"status value used as a fulfilment", "?fulfilment=delivered"},
		{"fulfilment is only commas", "?fulfilment=,"},
		{"limit zero", "?limit=0"},
		{"limit negative", "?limit=-1"},
		{"limit above the cap", "?limit=201"},
		{"limit not a number", "?limit=fifty"},
		{"before is not a timestamp", "?before=yesterday"},
		{"before is a date without a time", "?before=2026-08-14"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &stubAdminStore{}
			h, tok := newAdminOrderAPI(t, store)

			rec := getAdmin(t, h, "/api/v1/admin/orders"+tt.query, tok)

			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422 (%s)", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "VALIDATION_ERROR") {
				t.Errorf("body = %s, want a VALIDATION_ERROR envelope", rec.Body.String())
			}
			// A rejected filter must not reach the database.
			if store.calls != 0 {
				t.Errorf("store was called %d times for an invalid filter", store.calls)
			}
		})
	}
}

/* ----------------------------------------------------------------- paging */

func TestAdminOrdersOffersACursorOnlyOnAFullPage(t *testing.T) {
	last := time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		count      int
		limit      int
		wantCursor bool
	}{
		// A full page means there may be more.
		{"full page", 2, 2, true},
		// A short page is the last one. Offering a cursor would invite an
		// operator to page into nothing.
		{"short page", 1, 2, false},
		{"empty page", 0, 2, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orders := make([]order.Order, tt.count)
			for i := range orders {
				orders[i] = adminOrderFixture("EFF-10000"+string(rune('0'+i)),
					last.Add(time.Duration(tt.count-i)*time.Hour))
			}
			if tt.count > 0 {
				orders[tt.count-1].CreatedAt = last
			}
			store := &stubAdminStore{orders: orders}
			h, tok := newAdminOrderAPI(t, store)

			rec := getAdmin(t, h, "/api/v1/admin/orders?limit="+strconv.Itoa(tt.limit), tok)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
			}
			body := decodeAdminList(t, rec)

			if body.Data.Count != tt.count {
				t.Errorf("count = %d, want %d", body.Data.Count, tt.count)
			}
			if tt.wantCursor {
				if body.Data.NextBefore == nil {
					t.Fatal("nextBefore is absent on a full page")
				}
				// The cursor must be the OLDEST row returned, or the next page
				// re-serves rows this one already showed.
				if !body.Data.NextBefore.Equal(last) {
					t.Errorf("nextBefore = %v, want the last row's createdAt %v",
						body.Data.NextBefore, last)
				}
			} else if body.Data.NextBefore != nil {
				t.Errorf("nextBefore = %v, want absent", body.Data.NextBefore)
			}
		})
	}
}

/* -------------------------------------------------------------------- DTO */

func TestAdminOrderObjectCarriesTheOperatorFields(t *testing.T) {
	created := time.Date(2026, 8, 7, 10, 32, 11, 0, time.UTC)
	o := adminOrderFixture("EFF-483413", created)
	o.Fulfilment = order.FulfilmentPacked
	store := &stubAdminStore{orders: []order.Order{o}}
	h, tok := newAdminOrderAPI(t, store)

	rec := getAdmin(t, h, "/api/v1/admin/orders", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	got := decodeAdminList(t, rec).Data.Orders[0]

	checks := map[string]any{
		"orderId":           "EFF-483413",
		"status":            "placed",
		"statusLabel":       "Placed",
		"fulfilment":        "packed",
		"fulfilmentLabel":   "Processed",
		"razorpayOrderId":   "order_Pk9",
		"razorpayPaymentId": "pay_Pk9",
	}
	for k, want := range checks {
		if got[k] != want {
			t.Errorf("%s = %v, want %v", k, got[k], want)
		}
	}

	customer, ok := got["customer"].(map[string]any)
	if !ok {
		t.Fatalf("customer = %v, want an object", got["customer"])
	}
	if customer["phone"] != "9811111111" {
		t.Errorf("customer.phone = %v, want the frozen delivery number", customer["phone"])
	}
	if customer["userId"] == "" || customer["userId"] == nil {
		t.Error("customer.userId is missing")
	}

	// The signature is kept on the document for audit and has no operational
	// use. A console has no reason to hold an HMAC, so it must not ship.
	if strings.Contains(rec.Body.String(), "9f2b3c-secret-hmac") {
		t.Error("the response leaked razorpaySignature")
	}
	if strings.Contains(rec.Body.String(), "razorpaySignature") {
		t.Error("the response carries a razorpaySignature field")
	}
}

func TestAdminOrderObjectOnAnUntouchedOrder(t *testing.T) {
	// An order nobody has picked up yet: no fulfilment key at all.
	o := adminOrderFixture("EFF-483414", time.Now().UTC())
	o.Fulfilment = order.FulfilmentNone
	o.CustomerPhone = "" // and one placed before the phone was frozen
	store := &stubAdminStore{orders: []order.Order{o}}
	h, tok := newAdminOrderAPI(t, store)

	rec := getAdmin(t, h, "/api/v1/admin/orders", tok)
	got := decodeAdminList(t, rec).Data.Orders[0]

	// null rather than "", so a console can tell "not started" from a blank
	// string it might render as an empty chip.
	if got["fulfilment"] != nil {
		t.Errorf("fulfilment = %v, want null", got["fulfilment"])
	}
	if got["fulfilmentLabel"] != "Not started" {
		t.Errorf("fulfilmentLabel = %v, want %q", got["fulfilmentLabel"], "Not started")
	}
	customer := got["customer"].(map[string]any)
	if customer["phone"] != nil {
		t.Errorf("customer.phone = %v, want null on a pre-11.10 order", customer["phone"])
	}
}

// TestAdminOrderPaymentBlockMatchesTheShopperView pins that the payment block
// appears exactly when the shopper's own endpoint would show it. Gating on the
// method instead would make the block vanish for a placed order whose method
// never got stamped — hiding a data problem from the one person who could act
// on it. Real orders in Atlas are in that state, which is how this was found.
func TestAdminOrderPaymentBlockMatchesTheShopperView(t *testing.T) {
	t.Run("placed but with no method stamped", func(t *testing.T) {
		o := adminOrderFixture("EFF-483415", time.Now().UTC())
		o.Payment.Method = ""
		o.Payment.Label = ""
		o.Payment.VPA = ""
		store := &stubAdminStore{orders: []order.Order{o}}
		h, tok := newAdminOrderAPI(t, store)

		got := decodeAdminList(t, getAdmin(t, h, "/api/v1/admin/orders", tok)).Data.Orders[0]

		pay, ok := got["payment"].(map[string]any)
		if !ok {
			t.Fatalf("payment = %v, want an object even with no method", got["payment"])
		}
		if pay["method"] != nil {
			t.Errorf("payment.method = %v, want null", pay["method"])
		}
	})

	t.Run("not placed carries no payment block", func(t *testing.T) {
		o := adminOrderFixture("EFF-483416", time.Now().UTC())
		o.Status = order.StatusPendingPayment
		o.Fulfilment = order.FulfilmentNone
		store := &stubAdminStore{orders: []order.Order{o}}
		h, tok := newAdminOrderAPI(t, store)

		got := decodeAdminList(t, getAdmin(t, h, "/api/v1/admin/orders?status=all", tok)).Data.Orders[0]

		if _, present := got["payment"]; present {
			t.Errorf("payment = %v, want absent before money moved", got["payment"])
		}
	})
}

func TestAdminOrdersSurfacesStoreFailures(t *testing.T) {
	store := &stubAdminStore{err: errAdminBoom}
	h, tok := newAdminOrderAPI(t, store)

	rec := getAdmin(t, h, "/api/v1/admin/orders", tok)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (%s)", rec.Code, rec.Body.String())
	}
	// The driver error must not reach the client.
	if strings.Contains(rec.Body.String(), errAdminBoom.Error()) {
		t.Errorf("body = %s, want the database error hidden", rec.Body.String())
	}
}

func TestAdminOrdersRejectsTheWrongMethod(t *testing.T) {
	store := &stubAdminStore{}
	h, tok := newAdminOrderAPI(t, store)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/orders", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405 (%s)", rec.Code, rec.Body.String())
	}
}

/* -------------------------------------------------------------- test utils */

func sameStatuses(got, want []order.Status) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func sameFulfilments(got, want []order.Fulfilment) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

/* ------------------------------------------------- environment-split default */

// TestAdminOrdersDefaultSplitsByEnvironment pins the owner's 2026-08-17
// decision: production shows only orders somebody paid for, everywhere else
// shows every attempt so a failed one can be debugged without remembering
// ?status=all.
func TestAdminOrdersDefaultSplitsByEnvironment(t *testing.T) {
	tests := []struct {
		env      config.Environment
		wantPaid bool
	}{
		{config.EnvProduction, true},
		{config.EnvDevelopment, false},
		{config.EnvStaging, false},
		// An unset or unrecognised APP_ENV must NOT be treated as production
		// here. Failing open on a console behind an admin token shows an
		// operator more than they need; failing closed would hide a failed
		// payment from the developer debugging it, which is the whole point of
		// the split. Only the literal "production" restricts.
		{"", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.env), func(t *testing.T) {
			store := &stubAdminStore{}
			h, tok := newAdminOrderAPIIn(t, store, tt.env)

			rec := getAdmin(t, h, "/api/v1/admin/orders", tok)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
			}

			if tt.wantPaid {
				if !sameStatuses(store.got.Statuses, order.PaidStatuses()) {
					t.Errorf("statuses = %v, want the paid set", store.got.Statuses)
				}
				return
			}
			if store.got.Statuses != nil {
				t.Errorf("statuses = %v, want nil (every status) outside production",
					store.got.Statuses)
			}
		})
	}
}

// TestAdminOrdersExplicitStatusIgnoresTheEnvironment is the other half of the
// split, and the more important one: only the DEFAULT differs. A console that
// names what it wants must behave identically everywhere, or a filter verified
// in dev could quietly return something else in production.
func TestAdminOrdersExplicitStatusIgnoresTheEnvironment(t *testing.T) {
	for _, env := range []config.Environment{config.EnvProduction, config.EnvDevelopment} {
		t.Run(string(env)+"/explicit", func(t *testing.T) {
			store := &stubAdminStore{}
			h, tok := newAdminOrderAPIIn(t, store, env)

			if rec := getAdmin(t, h, "/api/v1/admin/orders?status=expired", tok); rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
			}
			if !sameStatuses(store.got.Statuses, []order.Status{order.StatusExpired}) {
				t.Errorf("statuses = %v, want [expired] in %s", store.got.Statuses, env)
			}
		})

		t.Run(string(env)+"/all", func(t *testing.T) {
			store := &stubAdminStore{}
			h, tok := newAdminOrderAPIIn(t, store, env)

			if rec := getAdmin(t, h, "/api/v1/admin/orders?status=all", tok); rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
			}
			if store.got.Statuses != nil {
				t.Errorf("statuses = %v, want nil for ?status=all in %s", store.got.Statuses, env)
			}
		})
	}
}

/* --------------------------------------------------------------- get order */

func TestAdminGetOrderReturnsTheOrder(t *testing.T) {
	o := adminOrderFixture("EFF-483413", time.Date(2026, 8, 7, 10, 32, 11, 0, time.UTC))
	o.Fulfilment = order.FulfilmentInTransit
	store := &stubAdminStore{one: o}
	h, tok := newAdminOrderAPI(t, store)

	rec := getAdmin(t, h, "/api/v1/admin/orders/EFF-483413", tok)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if store.gotID != "EFF-483413" {
		t.Errorf("looked up %q, want EFF-483413", store.gotID)
	}

	var body struct {
		Data struct {
			Order map[string]any `json:"order"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding %s: %v", rec.Body.String(), err)
	}
	got := body.Data.Order

	// Same object the list returns — one shape, one mapper.
	for k, want := range map[string]any{
		"orderId":         "EFF-483413",
		"status":          "placed",
		"statusLabel":     "Placed",
		"fulfilment":      "in_transit",
		"fulfilmentLabel": "Transit",
	} {
		if got[k] != want {
			t.Errorf("%s = %v, want %v", k, got[k], want)
		}
	}
	if strings.Contains(rec.Body.String(), "razorpaySignature") {
		t.Error("the detail response leaked razorpaySignature")
	}
}

func TestAdminGetOrderNotFound(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		storeErr  error
		wantQuery bool
	}{
		{
			name: "no such order", path: "/api/v1/admin/orders/EFF-000001",
			storeErr: order.ErrOrderNotFound, wantQuery: true,
		},
		{
			// A malformed id cannot name a real order, so it is answered without
			// touching the database rather than as a 422 — there is no ownership
			// dimension here, only existence.
			name: "malformed id", path: "/api/v1/admin/orders/nonsense",
			wantQuery: false,
		},
		{
			name: "id with the wrong digit count", path: "/api/v1/admin/orders/EFF-42",
			wantQuery: false,
		},
		{
			name: "lowercase prefix", path: "/api/v1/admin/orders/eff-483413",
			wantQuery: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &stubAdminStore{oneErr: tt.storeErr}
			h, tok := newAdminOrderAPI(t, store)

			rec := getAdmin(t, h, tt.path, tok)

			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404 (%s)", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "NOT_FOUND") {
				t.Errorf("body = %s, want a NOT_FOUND envelope", rec.Body.String())
			}
			if got := store.idCalls > 0; got != tt.wantQuery {
				t.Errorf("queried = %v, want %v", got, tt.wantQuery)
			}
		})
	}
}

func TestAdminGetOrderRequiresAnAdminToken(t *testing.T) {
	store := &stubAdminStore{one: adminOrderFixture("EFF-483413", time.Now().UTC())}
	h, _ := newAdminOrderAPI(t, store)

	rec := getAdmin(t, h, "/api/v1/admin/orders/EFF-483413", "")

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 (%s)", rec.Code, rec.Body.String())
	}
	if store.idCalls != 0 {
		t.Errorf("store was called %d times behind a 401", store.idCalls)
	}
}

func TestAdminGetOrderSurfacesStoreFailures(t *testing.T) {
	store := &stubAdminStore{oneErr: errAdminBoom}
	h, tok := newAdminOrderAPI(t, store)

	rec := getAdmin(t, h, "/api/v1/admin/orders/EFF-483413", tok)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (%s)", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), errAdminBoom.Error()) {
		t.Errorf("body = %s, want the database error hidden", rec.Body.String())
	}
}

/* --------------------------------------------------------- set fulfilment */

func patchAdmin(t *testing.T, h http.Handler, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func placedWithFulfilment(f order.Fulfilment) order.Order {
	o := adminOrderFixture("EFF-483413", time.Now().UTC())
	o.Fulfilment = f
	return o
}

func TestAdminSetFulfilmentAdvancesOneStep(t *testing.T) {
	tests := []struct {
		from, to order.Fulfilment
		wantLbl  string
	}{
		{order.FulfilmentNone, order.FulfilmentPacked, "Processed"},
		{order.FulfilmentPacked, order.FulfilmentInTransit, "Transit"},
		{order.FulfilmentInTransit, order.FulfilmentShipped, "Shipped"},
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
			store := &stubAdminStore{one: placedWithFulfilment(tt.from)}
			h, tok := newAdminOrderAPI(t, store)

			rec := patchAdmin(t, h, "/api/v1/admin/orders/EFF-483413/fulfilment", tok,
				`{"fulfilment":"`+string(tt.to)+`"}`)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
			}
			if len(store.setCalls) != 1 {
				t.Fatalf("%d writes, want 1", len(store.setCalls))
			}
			// The FROM state must be passed through as the guard. Without it a
			// concurrent operator could advance the same order twice.
			got := store.setCalls[0]
			if got.from != tt.from || got.to != tt.to {
				t.Errorf("wrote from=%q to=%q, want from=%q to=%q", got.from, got.to, tt.from, tt.to)
			}

			var body struct {
				Data struct {
					Order map[string]any `json:"order"`
				} `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decoding %s: %v", rec.Body.String(), err)
			}
			// The response must show the NEW state, not the one just read.
			if body.Data.Order["fulfilment"] != string(tt.to) {
				t.Errorf("fulfilment = %v, want %q", body.Data.Order["fulfilment"], tt.to)
			}
			if body.Data.Order["fulfilmentLabel"] != tt.wantLbl {
				t.Errorf("fulfilmentLabel = %v, want %q", body.Data.Order["fulfilmentLabel"], tt.wantLbl)
			}
		})
	}
}

func TestAdminSetFulfilmentRejectsIllegalMoves(t *testing.T) {
	tests := []struct {
		name    string
		from    order.Fulfilment
		to      string
		wantMsg string
	}{
		{"skipping a step", order.FulfilmentNone, "shipped",
			"This order is Not started. The next step is Processed, not Shipped."},
		{"backwards", order.FulfilmentShipped, "packed",
			"This order is already Shipped, which is the last step."},
		{"self-move", order.FulfilmentPacked, "packed",
			"This order is already Processed."},
		{"past the end", order.FulfilmentShipped, "shipped",
			"This order is already Shipped."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &stubAdminStore{one: placedWithFulfilment(tt.from)}
			h, tok := newAdminOrderAPI(t, store)

			rec := patchAdmin(t, h, "/api/v1/admin/orders/EFF-483413/fulfilment", tok,
				`{"fulfilment":"`+tt.to+`"}`)

			if rec.Code != http.StatusConflict {
				t.Fatalf("status = %d, want 409 (%s)", rec.Code, rec.Body.String())
			}
			// The message must name what the operator CAN do; "no" alone leaves
			// them guessing which button was right.
			if !strings.Contains(rec.Body.String(), tt.wantMsg) {
				t.Errorf("body = %s,\n want it to contain %q", rec.Body.String(), tt.wantMsg)
			}
			if len(store.setCalls) != 0 {
				t.Errorf("%d writes on a refused move, want 0", len(store.setCalls))
			}
		})
	}
}

func TestAdminSetFulfilmentRequiresAPlacedOrder(t *testing.T) {
	// Fulfilment progress on an order nobody paid for is a parcel going out for
	// free.
	for _, st := range []order.Status{
		order.StatusPendingPayment, order.StatusExpired, order.StatusPaymentFailed,
	} {
		t.Run(string(st), func(t *testing.T) {
			o := placedWithFulfilment(order.FulfilmentNone)
			o.Status = st
			store := &stubAdminStore{one: o}
			h, tok := newAdminOrderAPI(t, store)

			rec := patchAdmin(t, h, "/api/v1/admin/orders/EFF-483413/fulfilment", tok,
				`{"fulfilment":"packed"}`)

			if rec.Code != http.StatusConflict {
				t.Fatalf("status = %d, want 409 (%s)", rec.Code, rec.Body.String())
			}
			if len(store.setCalls) != 0 {
				t.Errorf("%d writes on an unpaid order, want 0", len(store.setCalls))
			}
		})
	}
}

func TestAdminSetFulfilmentConcurrentChangeIsAConflict(t *testing.T) {
	// The guard matched nothing: somebody else moved this order between the read
	// and the write. Answering 200 would tell this operator they advanced it
	// when another person actually did.
	store := &stubAdminStore{one: placedWithFulfilment(order.FulfilmentNone), setNotModified: true}
	h, tok := newAdminOrderAPI(t, store)

	rec := patchAdmin(t, h, "/api/v1/admin/orders/EFF-483413/fulfilment", tok,
		`{"fulfilment":"packed"}`)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Somebody else changed this order") {
		t.Errorf("body = %s, want the concurrent-change message", rec.Body.String())
	}
}

func TestAdminSetFulfilmentBadRequests(t *testing.T) {
	tests := []struct {
		name, path, body string
		wantCode         int
	}{
		{"missing field", "/api/v1/admin/orders/EFF-483413/fulfilment", `{}`, http.StatusUnprocessableEntity},
		{"blank value", "/api/v1/admin/orders/EFF-483413/fulfilment", `{"fulfilment":""}`, http.StatusUnprocessableEntity},
		{"unknown value", "/api/v1/admin/orders/EFF-483413/fulfilment", `{"fulfilment":"delivered"}`, http.StatusUnprocessableEntity},
		{"a status, not a fulfilment", "/api/v1/admin/orders/EFF-483413/fulfilment", `{"fulfilment":"cancelled"}`, http.StatusUnprocessableEntity},
		{"none is not a target", "/api/v1/admin/orders/EFF-483413/fulfilment", `{"fulfilment":"none"}`, http.StatusUnprocessableEntity},
		{"unknown field", "/api/v1/admin/orders/EFF-483413/fulfilment", `{"status":"packed"}`, http.StatusBadRequest},
		{"malformed json", "/api/v1/admin/orders/EFF-483413/fulfilment", `{`, http.StatusBadRequest},
		{"malformed order id", "/api/v1/admin/orders/nonsense/fulfilment", `{"fulfilment":"packed"}`, http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &stubAdminStore{one: placedWithFulfilment(order.FulfilmentNone)}
			h, tok := newAdminOrderAPI(t, store)

			rec := patchAdmin(t, h, tt.path, tok, tt.body)

			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tt.wantCode, rec.Body.String())
			}
			if len(store.setCalls) != 0 {
				t.Errorf("%d writes on a rejected request, want 0", len(store.setCalls))
			}
		})
	}
}

func TestAdminSetFulfilmentRequiresAnAdminToken(t *testing.T) {
	store := &stubAdminStore{one: placedWithFulfilment(order.FulfilmentNone)}
	h, _ := newAdminOrderAPI(t, store)

	rec := patchAdmin(t, h, "/api/v1/admin/orders/EFF-483413/fulfilment", "",
		`{"fulfilment":"packed"}`)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 (%s)", rec.Code, rec.Body.String())
	}
	if len(store.setCalls) != 0 || store.idCalls != 0 {
		t.Error("the store was reached behind a 401")
	}
}

func TestAdminSetFulfilmentSurfacesWriteFailures(t *testing.T) {
	store := &stubAdminStore{one: placedWithFulfilment(order.FulfilmentNone), setErr: errAdminBoom}
	h, tok := newAdminOrderAPI(t, store)

	rec := patchAdmin(t, h, "/api/v1/admin/orders/EFF-483413/fulfilment", tok,
		`{"fulfilment":"packed"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (%s)", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), errAdminBoom.Error()) {
		t.Errorf("body = %s, want the database error hidden", rec.Body.String())
	}
}

/* -------------------------------------------------------------------- label */

// newAdminOrderAPIWithOrigin builds the API with a specific shipping origin, so
// the unconfigured path can be exercised.
func newAdminOrderAPIWithOrigin(
	t *testing.T, store *stubAdminStore, origin config.ShipFromAddress,
) (http.Handler, string) {
	t.Helper()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	issuer := admin.NewTokenIssuer(adminOrderSecret, admin.TokenTTL)
	h := server.New(server.Deps{
		Config:     config.Config{},
		Mongo:      stubAdminPinger{},
		AdminOrder: order.NewAdminHandler(store, adminTokenParser{issuer}, logger, config.EnvProduction, origin),
		Logger:     logger,
		Version:    "test",
		Started:    time.Now(),
	})
	tok, _, err := issuer.Issue("ops@enerzia.in")
	if err != nil {
		t.Fatalf("issue admin token: %v", err)
	}
	return h, tok
}

func TestAdminLabelRendersTheOrder(t *testing.T) {
	o := adminOrderFixture("EFF-483413", time.Date(2026, 8, 7, 10, 32, 11, 0, time.UTC))
	store := &stubAdminStore{one: o}
	h, tok := newAdminOrderAPI(t, store)

	rec := getAdmin(t, h, "/api/v1/admin/orders/EFF-483413/label", tok)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	// A label is a snapshot; a cached copy could be printed after the address
	// behind it changed.
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}

	body := rec.Body.String()
	for _, want := range []string{
		"EFF-483413",
		"Ananya Sharma",         // recipient
		"12, Anand Residency",   // street
		"411001",                // recipient pin
		"9811111111",            // the FROZEN delivery contact
		"Enerzeia Future Farm",  // return address
		"121004",                // origin pin
		"9812345678",            // origin phone
		"Pure Spirulina Powder", // contents, so the packer can check the box
		"PREPAID",               // no COD in this shop
		"size: 4in 6in",         // the physical label size
	} {
		if !strings.Contains(body, want) {
			t.Errorf("label is missing %q", want)
		}
	}

	// Nothing about money or the gateway belongs on a parcel.
	for _, unwanted := range []string{"razorpay", "order_Pk9", "pay_Pk9", "65000", "a@b.co"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("label leaked %q", unwanted)
		}
	}
}

func TestAdminLabelEscapesShopperText(t *testing.T) {
	// The shipping address is shopper-supplied. html/template must escape it,
	// or a street line containing markup breaks the label — or worse.
	o := adminOrderFixture("EFF-483413", time.Now().UTC())
	o.ShippingAddress.Name = `<script>alert(1)</script>`
	o.ShippingAddress.Line1 = `12 "Quote" & <b>Bold</b> Road`
	store := &stubAdminStore{one: o}
	h, tok := newAdminOrderAPI(t, store)

	rec := getAdmin(t, h, "/api/v1/admin/orders/EFF-483413/label", tok)
	body := rec.Body.String()

	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Error("the label rendered a raw <script> from shopper text")
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Error("the script tag was not escaped")
	}
	if strings.Contains(body, "<b>Bold</b>") {
		t.Error("shopper markup was rendered rather than escaped")
	}
}

func TestAdminLabelWithoutAnOrigin(t *testing.T) {
	// 503, never a label with blanks where the return address belongs: a parcel
	// that can be neither delivered nor returned is simply gone.
	store := &stubAdminStore{one: adminOrderFixture("EFF-483413", time.Now().UTC())}
	h, tok := newAdminOrderAPIWithOrigin(t, store, config.ShipFromAddress{})

	rec := getAdmin(t, h, "/api/v1/admin/orders/EFF-483413/label", tok)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Shipping origin is not configured") {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestAdminLabelPartialOriginIsStillUnconfigured(t *testing.T) {
	// All-or-nothing. A missing PIN is exactly the line that gets an
	// undelivered parcel home, and printing without it silently is the failure
	// this guards against.
	partial := testOrigin
	partial.Pin = ""
	store := &stubAdminStore{one: adminOrderFixture("EFF-483413", time.Now().UTC())}
	h, tok := newAdminOrderAPIWithOrigin(t, store, partial)

	rec := getAdmin(t, h, "/api/v1/admin/orders/EFF-483413/label", tok)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 (%s)", rec.Code, rec.Body.String())
	}
}

func TestAdminLabelRefusesUnpaidOrders(t *testing.T) {
	for _, st := range []order.Status{
		order.StatusPendingPayment, order.StatusExpired, order.StatusPaymentFailed,
	} {
		t.Run(string(st), func(t *testing.T) {
			o := adminOrderFixture("EFF-483413", time.Now().UTC())
			o.Status = st
			store := &stubAdminStore{one: o}
			h, tok := newAdminOrderAPI(t, store)

			rec := getAdmin(t, h, "/api/v1/admin/orders/EFF-483413/label", tok)

			if rec.Code != http.StatusConflict {
				t.Fatalf("status = %d, want 409 (%s)", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestAdminLabelNotFoundAndAuth(t *testing.T) {
	t.Run("unknown order", func(t *testing.T) {
		store := &stubAdminStore{oneErr: order.ErrOrderNotFound}
		h, tok := newAdminOrderAPI(t, store)
		if rec := getAdmin(t, h, "/api/v1/admin/orders/EFF-000001/label", tok); rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})
	t.Run("malformed id", func(t *testing.T) {
		store := &stubAdminStore{}
		h, tok := newAdminOrderAPI(t, store)
		if rec := getAdmin(t, h, "/api/v1/admin/orders/nope/label", tok); rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
		if store.idCalls != 0 {
			t.Error("a malformed id reached the database")
		}
	})
	t.Run("no token", func(t *testing.T) {
		store := &stubAdminStore{one: adminOrderFixture("EFF-483413", time.Now().UTC())}
		h, _ := newAdminOrderAPI(t, store)
		if rec := getAdmin(t, h, "/api/v1/admin/orders/EFF-483413/label", ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
		if store.idCalls != 0 {
			t.Error("the store was reached behind a 401")
		}
	})
}

func TestAdminLabelOmitsThePhoneLineWhenAbsent(t *testing.T) {
	// An order placed before customerPhone existed has none. The label must
	// simply not carry a phone line rather than print an empty bold row.
	o := adminOrderFixture("EFF-483413", time.Now().UTC())
	o.CustomerPhone = ""
	store := &stubAdminStore{one: o}
	h, tok := newAdminOrderAPI(t, store)

	rec := getAdmin(t, h, "/api/v1/admin/orders/EFF-483413/label", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if strings.Contains(rec.Body.String(), `class="phone"`) {
		t.Error("an empty phone row was printed")
	}
}
