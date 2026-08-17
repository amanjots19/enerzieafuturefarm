package order_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/enerzia/enerzia-be/internal/auth"
	"github.com/enerzia/enerzia-be/internal/cart"
	"github.com/enerzia/enerzia-be/internal/catalogue"
	"github.com/enerzia/enerzia-be/internal/email"
	"github.com/enerzia/enerzia-be/internal/order"
	"github.com/enerzia/enerzia-be/internal/razorpay"
)

/* ================================================================ fakes */

// fakeStore is an in-memory order.Store for service tests. It does not
// exercise MongoDB semantics — that is repository_test.go's job.
type fakeStore struct {
	created          []order.Order
	createErr        error
	expireOrder      order.Order
	expireFound      bool
	expireErr        error
	setRzpErr        error
	byIDOrder        order.Order
	byIDErr          error
	listOrders       []order.Order
	listErr          error
	byRzpOrder       order.Order
	byRzpErr         error
	markModified     bool
	markPlacedErr    error
	markFailModified bool
	markFailErr      error

	fillModified bool
	fillErr      error
	filled       []order.Payment
}

func (f *fakeStore) FillPaymentDetail(_ context.Context, _ string, p order.Payment, _ time.Time) (bool, error) {
	if f.fillErr != nil {
		return false, f.fillErr
	}
	f.filled = append(f.filled, p)
	return f.fillModified, nil
}

func (f *fakeStore) Create(_ context.Context, o order.Order) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = append(f.created, o)
	return nil
}

func (f *fakeStore) ByOrderID(_ context.Context, _ bson.ObjectID, _ string) (order.Order, error) {
	return f.byIDOrder, f.byIDErr
}

func (f *fakeStore) ListForUser(_ context.Context, _ bson.ObjectID) ([]order.Order, error) {
	return f.listOrders, f.listErr
}

func (f *fakeStore) ByRazorpayOrderID(_ context.Context, _ string) (order.Order, error) {
	return f.byRzpOrder, f.byRzpErr
}

func (f *fakeStore) MarkPlaced(_ context.Context, _ string, _ order.Payment, _, _ time.Time) (bool, error) {
	return f.markModified, f.markPlacedErr
}

func (f *fakeStore) MarkPaymentFailed(_ context.Context, _ string, _ order.Payment, _ time.Time) (bool, error) {
	return f.markFailModified, f.markFailErr
}

func (f *fakeStore) ExpirePendingForUser(_ context.Context, _ bson.ObjectID, _ time.Time) (order.Order, bool, error) {
	return f.expireOrder, f.expireFound, f.expireErr
}

func (f *fakeStore) SetRazorpayOrderID(_ context.Context, _ string, _ string) error {
	return f.setRzpErr
}

// fakeEventStore is an in-memory PaymentEventStore for service tests.
type fakeEventStore struct {
	insertErr     error
	inserted      []order.PaymentEvent
	getEvent      order.PaymentEvent
	getErr        error
	markProcErr   error
	markedProcIDs []string
}

func (f *fakeEventStore) InsertEvent(_ context.Context, e order.PaymentEvent) error {
	if f.insertErr != nil {
		return f.insertErr
	}
	f.inserted = append(f.inserted, e)
	return nil
}

func (f *fakeEventStore) GetEvent(_ context.Context, _ string) (order.PaymentEvent, error) {
	return f.getEvent, f.getErr
}

func (f *fakeEventStore) MarkEventProcessed(_ context.Context, eventID string) error {
	if f.markProcErr != nil {
		return f.markProcErr
	}
	f.markedProcIDs = append(f.markedProcIDs, eventID)
	return nil
}

// fakeLiner is a CartLiner that returns preset lines or an error.
type fakeLiner struct {
	lines []cart.Line
	err   error
}

func (f *fakeLiner) Lines(_ context.Context, _ bson.ObjectID) ([]cart.Line, error) {
	return f.lines, f.err
}

// fakeAddr is an AddressResolver with a single address.
type fakeAddr struct {
	addr auth.Address
	// accountPhone is the shopper's OTP-proven number, used only when the
	// address carries none of its own.
	accountPhone string
	err          error
}

func (f *fakeAddr) AddressFor(_ context.Context, _ bson.ObjectID, _ *bson.ObjectID) (auth.Address, string, error) {
	return f.addr, f.accountPhone, f.err
}

// recordingNotifier captures sent messages. Send may run on another goroutine,
// so it is mutex-guarded and exposes a wait.
type recordingNotifier struct {
	mu   sync.Mutex
	sent []email.Message
	err  error
	done chan struct{}
}

func newRecordingNotifier() *recordingNotifier {
	return &recordingNotifier{done: make(chan struct{}, 8)}
}

func (n *recordingNotifier) Send(_ context.Context, m email.Message) error {
	n.mu.Lock()
	if n.err == nil {
		n.sent = append(n.sent, m)
	}
	n.mu.Unlock()
	n.done <- struct{}{}
	return n.err
}

func (n *recordingNotifier) count() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.sent)
}

// waitForSend blocks until one Send happens, or fails the test. The send is
// deliberately detached from the request, so a test cannot assume it has
// already run.
func (n *recordingNotifier) waitForSend(t *testing.T) {
	t.Helper()
	select {
	case <-n.done:
	case <-time.After(2 * time.Second):
		t.Fatal("no confirmation was sent within 2s")
	}
}

// fakeStock tracks TakeStock calls and can inject failures.
type fakeStock struct {
	takeErr   error
	failOnIdx int // index of the TakeStock call that should fail (-1 = none)
	taken     int
	returned  int
	returnErr error // injected into every ReturnStock call when non-nil
}

func newFakeStock() *fakeStock { return &fakeStock{failOnIdx: -1} }

func (f *fakeStock) TakeStock(_ context.Context, _ catalogue.ID, _ int) error {
	if f.failOnIdx >= 0 && f.taken == f.failOnIdx {
		return f.takeErr
	}
	f.taken++
	return nil
}

func (f *fakeStock) ReturnStock(_ context.Context, _ catalogue.ID, _ int) error {
	f.returned++
	return f.returnErr
}

// fakeGateway is a razorpay.Gateway that returns a preset order or error.
type fakeGateway struct {
	rzpOrder    razorpay.Order
	err         error
	verifyCBErr error // error to return from VerifyCallbackSignature
	verifyWHErr error // error to return from VerifyWebhookSignature
}

func (f *fakeGateway) CreateOrder(_ context.Context, _ razorpay.CreateOrderRequest) (razorpay.Order, error) {
	return f.rzpOrder, f.err
}
func (f *fakeGateway) VerifyCallbackSignature(_, _, _ string) error    { return f.verifyCBErr }
func (f *fakeGateway) VerifyWebhookSignature(_ []byte, _ string) error { return f.verifyWHErr }

// fakeCartClearer is a CartClearer that records calls and can inject errors.
type fakeCartClearer struct {
	cleared bool
	err     error
}

func (f *fakeCartClearer) Clear(_ context.Context, _ bson.ObjectID) (cart.View, error) {
	f.cleared = true
	return cart.View{}, f.err
}

/* ======================================================== helpers */

var (
	testUser   = bson.NewObjectID()
	svcAddress = auth.Address{
		ID:        bson.NewObjectID(),
		Name:      "Test User",
		Email:     "test@example.com",
		Phone:     "9811111111",
		Line1:     "123 Main Street",
		City:      "Mumbai",
		State:     "Maharashtra",
		Pin:       "400001",
		IsDefault: true,
	}
)

func goodLine(name string, price int64, qty int) cart.Line {
	return cart.Line{
		ProductID: catalogue.ID("prod-" + name),
		Name:      name,
		Form:      catalogue.FormPowder,
		Grad:      "#4ade80",
		UnitPrice: price,
		UnitMRP:   price + 1000,
		Qty:       qty,
		LineTotal: price * int64(qty),
		Stock:     100,
	}
}

var fixedNow = time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)

func newSvc(t *testing.T, store order.Store, liner *fakeLiner, addr *fakeAddr, stock *fakeStock, gw razorpay.Gateway, clearer ...order.CartClearer) *order.Service {
	t.Helper()
	var c order.CartClearer
	if len(clearer) > 0 {
		c = clearer[0]
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	return order.NewService(order.ServiceConfig{
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
}

/* ======================================================== tests */

func TestOpenCheckout_EmptyCart(t *testing.T) {
	svc := newSvc(t, &fakeStore{}, &fakeLiner{}, &fakeAddr{addr: svcAddress}, newFakeStock(), &fakeGateway{})
	_, err := svc.OpenCheckout(t.Context(), testUser, nil)
	if !errors.Is(err, order.ErrCartEmpty) {
		t.Fatalf("got %v, want ErrCartEmpty", err)
	}
}

func TestOpenCheckout_CartLinerError(t *testing.T) {
	boom := errors.New("db down")
	svc := newSvc(t, &fakeStore{}, &fakeLiner{err: boom}, &fakeAddr{addr: svcAddress}, newFakeStock(), &fakeGateway{})
	_, err := svc.OpenCheckout(t.Context(), testUser, nil)
	if !errors.Is(err, boom) {
		t.Fatalf("got %v, want wrapped boom", err)
	}
}

func TestOpenCheckout_BlockingLine(t *testing.T) {
	blocking := cart.Line{
		ProductID: "prod-x",
		Name:      "Spirulina Powder 100g",
		Form:      catalogue.FormPowder,
		Qty:       5,
		Stock:     2, // qty > stock → Blocking()
		UnitPrice: 49900,
		LineTotal: 249500,
	}
	svc := newSvc(t, &fakeStore{}, &fakeLiner{lines: []cart.Line{blocking}}, &fakeAddr{addr: svcAddress}, newFakeStock(), &fakeGateway{})
	_, err := svc.OpenCheckout(t.Context(), testUser, nil)
	var berr *order.BlockingLineError
	if !errors.As(err, &berr) {
		t.Fatalf("got %v, want *BlockingLineError", err)
	}
	if berr.Name != "Spirulina Powder 100g" {
		t.Errorf("Name = %q, want %q", berr.Name, "Spirulina Powder 100g")
	}
}

func TestOpenCheckout_NoSavedAddress(t *testing.T) {
	lines := []cart.Line{goodLine("Powder", 49900, 1)}
	svc := newSvc(t, &fakeStore{}, &fakeLiner{lines: lines},
		&fakeAddr{err: auth.ErrAddressNotFound}, // addressID == nil → ErrNoSavedAddress
		newFakeStock(), &fakeGateway{})
	_, err := svc.OpenCheckout(t.Context(), testUser, nil)
	if !errors.Is(err, order.ErrNoSavedAddress) {
		t.Fatalf("got %v, want ErrNoSavedAddress", err)
	}
}

func TestOpenCheckout_AddressNotFound(t *testing.T) {
	lines := []cart.Line{goodLine("Powder", 49900, 1)}
	specificID := bson.NewObjectID()
	svc := newSvc(t, &fakeStore{}, &fakeLiner{lines: lines},
		&fakeAddr{err: auth.ErrAddressNotFound},
		newFakeStock(), &fakeGateway{})
	_, err := svc.OpenCheckout(t.Context(), testUser, &specificID)
	if !errors.Is(err, order.ErrAddressNotFound) {
		t.Fatalf("got %v, want ErrAddressNotFound", err)
	}
}

func TestOpenCheckout_ExpiresOldOrder(t *testing.T) {
	lines := []cart.Line{goodLine("Powder", 49900, 1)}
	oldLine := order.Line{
		ProductID: "prod-old", Qty: 3, UnitPrice: 1, UnitMrp: 1,
		LineTotal: 3, Form: catalogue.FormPowder, Name: "old",
	}
	store := &fakeStore{
		expireFound: true,
		expireOrder: order.Order{Lines: []order.Line{oldLine}},
	}
	stock := newFakeStock()
	gw := &fakeGateway{rzpOrder: razorpay.Order{ID: "order_xxx", Amount: 49900, Currency: "INR"}}

	svc := newSvc(t, store, &fakeLiner{lines: lines}, &fakeAddr{addr: svcAddress}, stock, gw)
	if _, err := svc.OpenCheckout(t.Context(), testUser, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Stock was returned for the expired order (oldLine.Qty=3 → 1 ReturnStock call),
	// then taken once for the new order's single line.
	if stock.returned < 1 {
		t.Errorf("returned = %d, want ≥1 (expired order's stock)", stock.returned)
	}
}

// TestOpenCheckoutFreezesTheDeliveryPhone covers the whole fallback rule in one
// table. The address's own number is the delivery contact — for a gift that is
// the recipient, not the buyer — and the account number is used only when the
// address predates the per-address phone field.
func TestOpenCheckoutFreezesTheDeliveryPhone(t *testing.T) {
	tests := []struct {
		name         string
		addrPhone    string
		accountPhone string
		want         string
	}{
		{
			name:      "the address's own number wins",
			addrPhone: "9811111111", accountPhone: "9700000000", want: "9811111111",
		},
		{
			name: "an address saved before the field falls back to the account",
			// The buyer's own number is a worse guess than the recipient's, but
			// it is far better than a parcel with no contact at all.
			addrPhone: "", accountPhone: "9700000000", want: "9700000000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := svcAddress
			addr.Phone = tt.addrPhone
			store := &fakeStore{}
			gw := &fakeGateway{rzpOrder: razorpay.Order{ID: "order_x", Amount: 49900, Currency: "INR"}}

			svc := newSvc(t, store,
				&fakeLiner{lines: []cart.Line{goodLine("Powder", 49900, 1)}},
				&fakeAddr{addr: addr, accountPhone: tt.accountPhone},
				newFakeStock(), gw)

			if _, err := svc.OpenCheckout(t.Context(), testUser, nil); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(store.created) != 1 {
				t.Fatalf("created %d orders, want 1", len(store.created))
			}
			if got := store.created[0].CustomerPhone; got != tt.want {
				t.Errorf("CustomerPhone = %q, want %q", got, tt.want)
			}
			// The address snapshot keeps its own value untouched either way —
			// the frozen contact is a separate field, not a rewrite.
			if got := store.created[0].ShippingAddress.Phone; got != tt.addrPhone {
				t.Errorf("ShippingAddress.Phone = %q, want %q (unmodified)", got, tt.addrPhone)
			}
		})
	}
}

func TestOpenCheckout_StockUnavailable(t *testing.T) {
	lines := []cart.Line{
		goodLine("Powder", 49900, 2),
		goodLine("Tablets", 38000, 1),
	}
	stock := newFakeStock()
	stock.failOnIdx = 1 // second TakeStock fails
	stock.takeErr = catalogue.ErrOutOfStock

	svc := newSvc(t, &fakeStore{}, &fakeLiner{lines: lines}, &fakeAddr{addr: svcAddress}, stock, &fakeGateway{})
	_, err := svc.OpenCheckout(t.Context(), testUser, nil)
	var serr *order.StockUnavailableError
	if !errors.As(err, &serr) {
		t.Fatalf("got %v, want *StockUnavailableError", err)
	}
	if serr.Name != "Tablets" {
		t.Errorf("Name = %q, want Tablets", serr.Name)
	}
	// First line's stock must have been returned.
	if stock.returned != 1 {
		t.Errorf("returned = %d, want 1 (first line compensation)", stock.returned)
	}
}

func TestOpenCheckout_PendingOrderExists(t *testing.T) {
	lines := []cart.Line{goodLine("Powder", 49900, 1)}
	store := &fakeStore{createErr: order.ErrPendingOrderExists}
	stock := newFakeStock()

	svc := newSvc(t, store, &fakeLiner{lines: lines}, &fakeAddr{addr: svcAddress}, stock, &fakeGateway{})
	_, err := svc.OpenCheckout(t.Context(), testUser, nil)
	if !errors.Is(err, order.ErrPendingOrderExists) {
		t.Fatalf("got %v, want ErrPendingOrderExists", err)
	}
	// The reserved stock must have been returned.
	if stock.returned != 1 {
		t.Errorf("returned = %d, want 1", stock.returned)
	}
}

func TestOpenCheckout_GatewayFailure(t *testing.T) {
	lines := []cart.Line{goodLine("Powder", 49900, 2)}
	store := &fakeStore{}
	stock := newFakeStock()
	gw := &fakeGateway{err: errors.New("razorpay 502")}

	svc := newSvc(t, store, &fakeLiner{lines: lines}, &fakeAddr{addr: svcAddress}, stock, gw)
	_, err := svc.OpenCheckout(t.Context(), testUser, nil)
	if !errors.Is(err, order.ErrGateway) {
		t.Fatalf("got %v, want ErrGateway", err)
	}
	// Stock must have been returned for the single line (qty=2, but one TakeStock call).
	if stock.returned != 1 {
		t.Errorf("returned = %d, want 1 (one ReturnStock per line)", stock.returned)
	}
}

func TestOpenCheckout_SetRazorpayIDFailure(t *testing.T) {
	lines := []cart.Line{goodLine("Powder", 49900, 1)}
	store := &fakeStore{setRzpErr: errors.New("mongo timeout")}
	stock := newFakeStock()
	gw := &fakeGateway{rzpOrder: razorpay.Order{ID: "order_abc", Amount: 49900, Currency: "INR"}}

	svc := newSvc(t, store, &fakeLiner{lines: lines}, &fakeAddr{addr: svcAddress}, stock, gw)
	_, err := svc.OpenCheckout(t.Context(), testUser, nil)
	if !errors.Is(err, order.ErrGateway) {
		t.Fatalf("got %v, want ErrGateway on SetRazorpayOrderID failure", err)
	}
	// Stock must be returned even when SetRazorpayOrderID fails.
	if stock.returned != 1 {
		t.Errorf("returned = %d, want 1", stock.returned)
	}
}

func TestOpenCheckout_IDCollisionRetry(t *testing.T) {
	lines := []cart.Line{goodLine("Powder", 49900, 1)}
	store := &retryStore{failN: 2}
	stock := newFakeStock()
	gw := &fakeGateway{rzpOrder: razorpay.Order{ID: "order_retry", Amount: 49900, Currency: "INR"}}

	svc := newSvc(t, store, &fakeLiner{lines: lines}, &fakeAddr{addr: svcAddress}, stock, gw)
	result, err := svc.OpenCheckout(t.Context(), testUser, nil)
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if result.RazorpayOrderID != "order_retry" {
		t.Errorf("RazorpayOrderID = %q, want order_retry", result.RazorpayOrderID)
	}
	if store.calls < 3 {
		t.Errorf("Create called %d times, want ≥3", store.calls)
	}
}

// retryStore returns ErrDuplicateOrderID for the first failN calls to Create.
type retryStore struct {
	failN int
	calls int
}

func (r *retryStore) FillPaymentDetail(_ context.Context, _ string, _ order.Payment, _ time.Time) (bool, error) {
	return false, nil
}

func (r *retryStore) Create(_ context.Context, _ order.Order) error {
	r.calls++
	if r.calls <= r.failN {
		return order.ErrDuplicateOrderID
	}
	return nil
}
func (r *retryStore) ByOrderID(_ context.Context, _ bson.ObjectID, _ string) (order.Order, error) {
	return order.Order{}, nil
}
func (r *retryStore) ListForUser(_ context.Context, _ bson.ObjectID) ([]order.Order, error) {
	return nil, nil
}
func (r *retryStore) ByRazorpayOrderID(_ context.Context, _ string) (order.Order, error) {
	return order.Order{}, nil
}
func (r *retryStore) MarkPlaced(_ context.Context, _ string, _ order.Payment, _, _ time.Time) (bool, error) {
	return true, nil
}
func (r *retryStore) MarkPaymentFailed(_ context.Context, _ string, _ order.Payment, _ time.Time) (bool, error) {
	return false, nil
}
func (r *retryStore) ExpirePendingForUser(_ context.Context, _ bson.ObjectID, _ time.Time) (order.Order, bool, error) {
	return order.Order{}, false, nil
}
func (r *retryStore) SetRazorpayOrderID(_ context.Context, _ string, _ string) error {
	return nil
}

func TestOpenCheckout_Success(t *testing.T) {
	lines := []cart.Line{goodLine("Powder", 49900, 2)}
	store := &fakeStore{}
	stock := newFakeStock()
	gw := &fakeGateway{rzpOrder: razorpay.Order{ID: "order_success", Amount: 99800, Currency: "INR"}}

	svc := newSvc(t, store, &fakeLiner{lines: lines}, &fakeAddr{addr: svcAddress}, stock, gw)
	result, err := svc.OpenCheckout(t.Context(), testUser, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.RazorpayKeyID != "rzp_test_KEY" {
		t.Errorf("RazorpayKeyID = %q, want rzp_test_KEY", result.RazorpayKeyID)
	}
	if result.RazorpayOrderID != "order_success" {
		t.Errorf("RazorpayOrderID = %q, want order_success", result.RazorpayOrderID)
	}
	if result.Amount != 99800 {
		t.Errorf("Amount = %d, want 99800", result.Amount)
	}
	if result.Currency != "INR" {
		t.Errorf("Currency = %q, want INR", result.Currency)
	}
	if result.Order.Status != order.StatusPendingPayment {
		t.Errorf("Order.Status = %q, want pending_payment", result.Order.Status)
	}
	wantWindow := order.ReservationWindow
	if got := result.Order.ExpiresAt.Sub(result.Order.CreatedAt); got != wantWindow {
		t.Errorf("ExpiresAt - CreatedAt = %v, want %v", got, wantWindow)
	}
	// Stock was taken for 1 call (one line, qty=2 in a single TakeStock call).
	if stock.taken != 1 {
		t.Errorf("taken = %d, want 1", stock.taken)
	}
	if stock.returned != 0 {
		t.Errorf("returned = %d, want 0", stock.returned)
	}
}

/* ======================================================== ConfirmPayment tests */

// pendingOrder builds a minimal pending_payment order with a known razorpayOrderId.
func pendingOrder(razorpayOrderID string) order.Order {
	return order.Order{
		OrderID: "EFF-000001",
		UserID:  testUser,
		Status:  order.StatusPendingPayment,
		Payment: order.Payment{
			Provider:        "razorpay",
			Status:          order.PaymentStatusCreated,
			Amount:          49900,
			Currency:        "INR",
			RazorpayOrderID: razorpayOrderID,
		},
	}
}

func TestConfirmPayment_OrderNotFound(t *testing.T) {
	store := &fakeStore{byIDErr: order.ErrOrderNotFound}
	svc := newSvc(t, store, &fakeLiner{}, &fakeAddr{}, newFakeStock(), &fakeGateway{})
	_, err := svc.ConfirmPayment(t.Context(), testUser, "EFF-000001", "order_x", "pay_x", "sig_x")
	if !errors.Is(err, order.ErrOrderNotFound) {
		t.Fatalf("got %v, want ErrOrderNotFound", err)
	}
}

func TestConfirmPayment_AlreadyPlaced(t *testing.T) {
	placed := order.Order{OrderID: "EFF-000002", Status: order.StatusPlaced}
	store := &fakeStore{byIDOrder: placed}
	svc := newSvc(t, store, &fakeLiner{}, &fakeAddr{}, newFakeStock(), &fakeGateway{})
	got, err := svc.ConfirmPayment(t.Context(), testUser, "EFF-000002", "order_x", "pay_x", "sig_x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != order.StatusPlaced {
		t.Errorf("status = %q, want placed", got.Status)
	}
}

func TestConfirmPayment_OrderExpired(t *testing.T) {
	expired := order.Order{OrderID: "EFF-000003", Status: order.StatusExpired}
	store := &fakeStore{byIDOrder: expired}
	svc := newSvc(t, store, &fakeLiner{}, &fakeAddr{}, newFakeStock(), &fakeGateway{})
	_, err := svc.ConfirmPayment(t.Context(), testUser, "EFF-000003", "order_x", "pay_x", "sig_x")
	if !errors.Is(err, order.ErrOrderNotPending) {
		t.Fatalf("got %v, want ErrOrderNotPending", err)
	}
}

func TestConfirmPayment_SignatureInvalid(t *testing.T) {
	store := &fakeStore{byIDOrder: pendingOrder("order_real")}
	gw := &fakeGateway{verifyCBErr: errors.New("hmac mismatch")}
	svc := newSvc(t, store, &fakeLiner{}, &fakeAddr{}, newFakeStock(), gw)
	_, err := svc.ConfirmPayment(t.Context(), testUser, "EFF-000004", "order_real", "pay_x", "bad_sig")
	if !errors.Is(err, order.ErrSignatureInvalid) {
		t.Fatalf("got %v, want ErrSignatureInvalid", err)
	}
}

func TestConfirmPayment_RazorpayOrderIDMismatch(t *testing.T) {
	store := &fakeStore{byIDOrder: pendingOrder("order_stored")}
	svc := newSvc(t, store, &fakeLiner{}, &fakeAddr{}, newFakeStock(), &fakeGateway{})
	_, err := svc.ConfirmPayment(t.Context(), testUser, "EFF-000005", "order_different", "pay_x", "sig_x")
	if !errors.Is(err, order.ErrSignatureInvalid) {
		t.Fatalf("got %v, want ErrSignatureInvalid (id mismatch)", err)
	}
}

func TestConfirmPayment_MarkPlacedError(t *testing.T) {
	store := &fakeStore{
		byIDOrder:     pendingOrder("order_rz"),
		markPlacedErr: errors.New("mongo timeout"),
	}
	svc := newSvc(t, store, &fakeLiner{}, &fakeAddr{}, newFakeStock(), &fakeGateway{})
	_, err := svc.ConfirmPayment(t.Context(), testUser, "EFF-000006", "order_rz", "pay_rz", "sig_rz")
	if err == nil {
		t.Fatal("expected error from MarkPlaced, got nil")
	}
}

// seqStore returns different orders on successive ByOrderID calls.
type seqStore struct {
	fakeStore
	calls int
	byIDs []order.Order
}

func (s *seqStore) ByOrderID(_ context.Context, _ bson.ObjectID, _ string) (order.Order, error) {
	defer func() { s.calls++ }()
	if s.calls < len(s.byIDs) {
		return s.byIDs[s.calls], nil
	}
	return order.Order{}, order.ErrOrderNotFound
}

func TestConfirmPayment_WebhookBeatUs(t *testing.T) {
	// First ByOrderID → pending; MarkPlaced → (false, nil); second ByOrderID → placed.
	placedO := order.Order{OrderID: "EFF-000007", Status: order.StatusPlaced}
	store := &seqStore{
		byIDs: []order.Order{pendingOrder("order_rz"), placedO},
		fakeStore: fakeStore{
			markModified: false, // webhook already did it
		},
	}
	svc := newSvc(t, store, &fakeLiner{}, &fakeAddr{}, newFakeStock(), &fakeGateway{})
	got, err := svc.ConfirmPayment(t.Context(), testUser, "EFF-000007", "order_rz", "pay_rz", "sig_rz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != order.StatusPlaced {
		t.Errorf("status = %q, want placed", got.Status)
	}
}

func TestConfirmPayment_CartClearError(t *testing.T) {
	store := &fakeStore{
		byIDOrder:    pendingOrder("order_rz"),
		markModified: true,
	}
	clearer := &fakeCartClearer{err: errors.New("cart db down")}
	svc := newSvc(t, store, &fakeLiner{}, &fakeAddr{}, newFakeStock(), &fakeGateway{}, clearer)
	// Cart clear error must NOT propagate — the order is placed successfully.
	got, err := svc.ConfirmPayment(t.Context(), testUser, "EFF-000008", "order_rz", "pay_rz", "sig_rz")
	if err != nil {
		t.Fatalf("unexpected error on cart-clear failure: %v", err)
	}
	if got.Status != order.StatusPlaced {
		t.Errorf("status = %q, want placed", got.Status)
	}
	if !clearer.cleared {
		t.Error("Clear was not called")
	}
}

func TestConfirmPayment_Success(t *testing.T) {
	store := &fakeStore{
		byIDOrder:    pendingOrder("order_rzp"),
		markModified: true,
	}
	clearer := &fakeCartClearer{}
	svc := newSvc(t, store, &fakeLiner{}, &fakeAddr{}, newFakeStock(), &fakeGateway{}, clearer)
	got, err := svc.ConfirmPayment(t.Context(), testUser, "EFF-000009", "order_rzp", "pay_rzp", "sig_rzp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != order.StatusPlaced {
		t.Errorf("status = %q, want placed", got.Status)
	}
	if got.Payment.RazorpayPaymentID != "pay_rzp" {
		t.Errorf("RazorpayPaymentID = %q, want pay_rzp", got.Payment.RazorpayPaymentID)
	}
	if got.PlacedAt == nil {
		t.Error("PlacedAt must be set on success")
	}
	if !clearer.cleared {
		t.Error("cart was not cleared")
	}
}

// seqExpireStore returns a different error on ExpirePendingForUser calls after
// the first one. Used to exercise the compensateAfterInsert error path without
// failing the earlier step-4 expire check.
type seqExpireStore struct {
	fakeStore
	expireCalls int
	expireErr2  error
}

func (s *seqExpireStore) ExpirePendingForUser(_ context.Context, _ bson.ObjectID, _ time.Time) (order.Order, bool, error) {
	s.expireCalls++
	if s.expireCalls >= 2 {
		return order.Order{}, false, s.expireErr2
	}
	return s.expireOrder, s.expireFound, s.expireErr
}

func TestOpenCheckout_TotalsMatchPaymentAmount(t *testing.T) {
	lines := []cart.Line{
		goodLine("Powder", 49900, 2),  // 99800
		goodLine("Tablets", 38000, 1), // 38000
	}
	store := &fakeStore{}
	stock := newFakeStock()
	gw := &fakeGateway{rzpOrder: razorpay.Order{ID: "order_totals", Amount: 137800, Currency: "INR"}}

	svc := newSvc(t, store, &fakeLiner{lines: lines}, &fakeAddr{addr: svcAddress}, stock, gw)
	result, err := svc.OpenCheckout(t.Context(), testUser, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Amount != result.Order.Totals.Total {
		t.Errorf("Amount %d != Order.Totals.Total %d", result.Amount, result.Order.Totals.Total)
	}
}

/* ======================================================== Error() coverage */

func TestBlockingLineError_Error(t *testing.T) {
	err := &order.BlockingLineError{Name: "Spirulina Powder 100g"}
	want := "order: blocking line: Spirulina Powder 100g"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestStockUnavailableError_Error(t *testing.T) {
	err := &order.StockUnavailableError{Name: "Tablets"}
	want := "order: stock unavailable: Tablets"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

/* ======================================================== NewService */

func TestNewService_DefaultsClock(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	// Without a Now func the service must default to time.Now without panicking.
	svc := order.NewService(order.ServiceConfig{Logger: logger})
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
}

/* ======================================================== OpenCheckout additional paths */

func TestOpenCheckout_AddressError(t *testing.T) {
	// A non-ErrAddressNotFound error from AddressFor wraps and returns directly.
	lines := []cart.Line{goodLine("Powder", 49900, 1)}
	dbErr := errors.New("db connection lost")
	svc := newSvc(t, &fakeStore{}, &fakeLiner{lines: lines}, &fakeAddr{err: dbErr}, newFakeStock(), &fakeGateway{})
	_, err := svc.OpenCheckout(t.Context(), testUser, nil)
	if !errors.Is(err, dbErr) {
		t.Fatalf("got %v, want wrapped dbErr", err)
	}
}

func TestOpenCheckout_ValidateFails(t *testing.T) {
	// A cart line with a wrong LineTotal produces an order that fails Validate.
	// Create must not be called and the reserved stock must be returned.
	badLine := cart.Line{
		ProductID: catalogue.ID("prod-bad"),
		Name:      "Bad Powder",
		Form:      catalogue.FormPowder,
		Grad:      "#4ade80",
		UnitPrice: 49900,
		UnitMRP:   59900,
		Qty:       2,
		LineTotal: 1, // wrong: 49900*2 = 99800
		Stock:     100,
	}
	store := &fakeStore{}
	stock := newFakeStock()
	svc := newSvc(t, store, &fakeLiner{lines: []cart.Line{badLine}}, &fakeAddr{addr: svcAddress}, stock, &fakeGateway{})
	_, err := svc.OpenCheckout(t.Context(), testUser, nil)
	if err == nil {
		t.Fatal("expected Validate() to reject inconsistent order, got nil")
	}
	if stock.returned != 1 {
		t.Errorf("returned = %d, want 1 (stock released on validate failure)", stock.returned)
	}
	if len(store.created) != 0 {
		t.Error("Create must not be called when Validate fails")
	}
}

func TestOpenCheckout_ReleaseLinesContinuesAfterError(t *testing.T) {
	// If ReturnStock fails on the first line, releaseLines must still attempt
	// the second line — a compensation loop must not abort on the first error.
	lines := []cart.Line{
		goodLine("Powder", 49900, 1),
		goodLine("Tablets", 38000, 1),
	}
	stock := newFakeStock()
	stock.returnErr = errors.New("redis connection refused")
	gw := &fakeGateway{err: errors.New("razorpay 502")}

	svc := newSvc(t, &fakeStore{}, &fakeLiner{lines: lines}, &fakeAddr{addr: svcAddress}, stock, gw)
	_, err := svc.OpenCheckout(t.Context(), testUser, nil)
	if !errors.Is(err, order.ErrGateway) {
		t.Fatalf("got %v, want ErrGateway", err)
	}
	// Both lines must have had ReturnStock attempted even though the first errored.
	if stock.returned != 2 {
		t.Errorf("returned = %d, want 2 (releaseLines must not abort on first error)", stock.returned)
	}
}

func TestOpenCheckout_CompensateExpireError(t *testing.T) {
	// ExpirePendingForUser failing inside compensateAfterInsert must be logged
	// and swallowed — the caller must still receive ErrGateway.
	lines := []cart.Line{goodLine("Powder", 49900, 2)}
	store := &seqExpireStore{expireErr2: errors.New("expire failed after insert")}
	gw := &fakeGateway{err: errors.New("razorpay 502")}

	svc := newSvc(t, store, &fakeLiner{lines: lines}, &fakeAddr{addr: svcAddress}, newFakeStock(), gw)
	_, err := svc.OpenCheckout(t.Context(), testUser, nil)
	if !errors.Is(err, order.ErrGateway) {
		t.Fatalf("got %v, want ErrGateway (compensate error must be swallowed)", err)
	}
}

func TestOpenCheckout_IDCollisionExhausted(t *testing.T) {
	// All maxIDRetries attempts collide → stock must be released and an error returned.
	lines := []cart.Line{goodLine("Powder", 49900, 1)}
	store := &retryStore{failN: 5} // maxIDRetries = 5
	stock := newFakeStock()

	svc := newSvc(t, store, &fakeLiner{lines: lines}, &fakeAddr{addr: svcAddress}, stock, &fakeGateway{})
	_, err := svc.OpenCheckout(t.Context(), testUser, nil)
	if err == nil {
		t.Fatal("expected error after exhausting all ID collision retries, got nil")
	}
	if stock.returned != 1 {
		t.Errorf("returned = %d, want 1 (stock must be released on collision exhaustion)", stock.returned)
	}
}

func TestOpenCheckout_ExpireError(t *testing.T) {
	// When the initial ExpirePendingForUser call fails the error must propagate.
	// No stock has been reserved at this point so nothing needs to be released.
	lines := []cart.Line{goodLine("Powder", 49900, 1)}
	dbErr := errors.New("mongo timeout")
	store := &fakeStore{expireErr: dbErr}
	svc := newSvc(t, store, &fakeLiner{lines: lines}, &fakeAddr{addr: svcAddress}, newFakeStock(), &fakeGateway{})
	_, err := svc.OpenCheckout(t.Context(), testUser, nil)
	if !errors.Is(err, dbErr) {
		t.Fatalf("got %v, want wrapped expireErr", err)
	}
}

func TestOpenCheckout_TakeStockGenericError(t *testing.T) {
	// A non-OutOfStock TakeStock error must be wrapped and returned directly.
	lines := []cart.Line{goodLine("Powder", 49900, 1)}
	dbErr := errors.New("redis timeout")
	stock := newFakeStock()
	stock.failOnIdx = 0
	stock.takeErr = dbErr
	svc := newSvc(t, &fakeStore{}, &fakeLiner{lines: lines}, &fakeAddr{addr: svcAddress}, stock, &fakeGateway{})
	_, err := svc.OpenCheckout(t.Context(), testUser, nil)
	if !errors.Is(err, dbErr) {
		t.Fatalf("got %v, want wrapped dbErr", err)
	}
}

func TestOpenCheckout_CreateGenericError(t *testing.T) {
	// A Create error that is neither ErrDuplicateOrderID nor ErrPendingOrderExists
	// must be wrapped and returned, with reserved stock released.
	lines := []cart.Line{goodLine("Powder", 49900, 1)}
	dbErr := errors.New("mongo timeout")
	store := &fakeStore{createErr: dbErr}
	stock := newFakeStock()
	svc := newSvc(t, store, &fakeLiner{lines: lines}, &fakeAddr{addr: svcAddress}, stock, &fakeGateway{})
	_, err := svc.OpenCheckout(t.Context(), testUser, nil)
	if !errors.Is(err, dbErr) {
		t.Fatalf("got %v, want wrapped dbErr", err)
	}
	if stock.returned != 1 {
		t.Errorf("returned = %d, want 1 (stock must be released on create error)", stock.returned)
	}
}

/* ======================================================== HandleWebhook tests */

const (
	webhookEvtID  = "evt_123456789"
	webhookRzpOID = "order_rzp"
	webhookPayID  = "pay_xxx"
	webhookAmount = int64(49900)
)

func capturePayload(rzpOID, payID string, amount int64) []byte {
	return []byte(fmt.Sprintf(
		`{"event":"payment.captured","payload":{"payment":{"entity":{"id":%q,"order_id":%q,"amount":%d,"method":"upi","vpa":"test@upi"}}}}`,
		payID, rzpOID, amount,
	))
}

func failedPayload(rzpOID, payID string) []byte {
	return []byte(fmt.Sprintf(
		`{"event":"payment.failed","payload":{"payment":{"entity":{"id":%q,"order_id":%q,"amount":49900,"error_code":"BAD_REQUEST_ERROR","error_description":"desc","error_source":"customer","error_step":"auth","error_reason":"abandoned"}}}}`,
		payID, rzpOID,
	))
}

// newWebhookSvc creates a service for HandleWebhook tests with an injectable event store.
func newWebhookSvc(t *testing.T, store order.Store, events order.PaymentEventStore, gw razorpay.Gateway, stock *fakeStock, clearer ...order.CartClearer) *order.Service {
	t.Helper()
	var c order.CartClearer
	if len(clearer) > 0 {
		c = clearer[0]
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	return order.NewService(order.ServiceConfig{
		Repo:          store,
		Cart:          &fakeLiner{},
		Auth:          &fakeAddr{},
		Catalogue:     stock,
		Gateway:       gw,
		Events:        events,
		CartClearer:   c,
		RazorpayKeyID: "rzp_test_KEY",
		Logger:        logger,
		Now:           func() time.Time { return fixedNow },
	})
}

func webhookOrder(total int64) order.Order {
	return order.Order{
		OrderID: "EFF-000001",
		UserID:  testUser,
		Status:  order.StatusPendingPayment,
		Totals:  order.Totals{Total: total},
		Payment: order.Payment{RazorpayOrderID: webhookRzpOID, Amount: total, Currency: "INR"},
	}
}

func TestHandleWebhook_InvalidSignature(t *testing.T) {
	gw := &fakeGateway{verifyWHErr: errors.New("hmac mismatch")}
	svc := newWebhookSvc(t, &fakeStore{}, &fakeEventStore{}, gw, newFakeStock())
	err := svc.HandleWebhook(t.Context(), capturePayload(webhookRzpOID, webhookPayID, webhookAmount), webhookEvtID, "bad_sig")
	if !errors.Is(err, order.ErrWebhookSignatureInvalid) {
		t.Fatalf("got %v, want ErrWebhookSignatureInvalid", err)
	}
}

func TestHandleWebhook_MalformedBody(t *testing.T) {
	events := &fakeEventStore{}
	svc := newWebhookSvc(t, &fakeStore{}, events, &fakeGateway{}, newFakeStock())
	err := svc.HandleWebhook(t.Context(), []byte(`not valid json`), webhookEvtID, "valid_sig")
	if err != nil {
		t.Fatalf("got %v, want nil (malformed body must be swallowed after verification)", err)
	}
	if len(events.inserted) != 0 {
		t.Error("malformed body must not be recorded in payment_events (parse fails before insert)")
	}
}

func TestHandleWebhook_DuplicateEventAlreadyProcessed(t *testing.T) {
	// Processed: true means a prior delivery completed dispatch successfully.
	events := &fakeEventStore{
		insertErr: order.ErrDuplicateEvent,
		getEvent:  order.PaymentEvent{Processed: true},
	}
	svc := newWebhookSvc(t, &fakeStore{}, events, &fakeGateway{}, newFakeStock())
	err := svc.HandleWebhook(t.Context(), capturePayload(webhookRzpOID, webhookPayID, webhookAmount), webhookEvtID, "sig")
	if err != nil {
		t.Fatalf("got %v, want nil (dup event with Processed:true is acknowledged)", err)
	}
}

func TestHandleWebhook_EventStoreError(t *testing.T) {
	dbErr := errors.New("mongo timeout")
	events := &fakeEventStore{insertErr: dbErr}
	svc := newWebhookSvc(t, &fakeStore{}, events, &fakeGateway{}, newFakeStock())
	err := svc.HandleWebhook(t.Context(), capturePayload(webhookRzpOID, webhookPayID, webhookAmount), webhookEvtID, "sig")
	if !errors.Is(err, dbErr) {
		t.Fatalf("got %v, want wrapped dbErr", err)
	}
}

func TestHandleWebhook_UnknownEvent(t *testing.T) {
	events := &fakeEventStore{}
	svc := newWebhookSvc(t, &fakeStore{}, events, &fakeGateway{}, newFakeStock())
	err := svc.HandleWebhook(t.Context(), []byte(`{"event":"subscription.charged","payload":{}}`), webhookEvtID, "sig")
	if err != nil {
		t.Fatalf("got %v, want nil (unknown events are acknowledged)", err)
	}
	if len(events.inserted) != 1 {
		t.Error("unknown event must still be recorded in payment_events")
	}
}

func TestHandleWebhook_CaptureNilPaymentWrapper(t *testing.T) {
	events := &fakeEventStore{}
	svc := newWebhookSvc(t, &fakeStore{}, events, &fakeGateway{}, newFakeStock())
	// payment.captured without a payment entity — razorpayOrderId will be empty.
	err := svc.HandleWebhook(t.Context(), []byte(`{"event":"payment.captured","payload":{}}`), webhookEvtID, "sig")
	if err != nil {
		t.Fatalf("got %v, want nil (empty razorpayOrderId skips lookup)", err)
	}
}

func TestHandleWebhook_CaptureOrderNotFound(t *testing.T) {
	store := &fakeStore{byRzpErr: order.ErrOrderNotFound}
	svc := newWebhookSvc(t, store, &fakeEventStore{}, &fakeGateway{}, newFakeStock())
	err := svc.HandleWebhook(t.Context(), capturePayload(webhookRzpOID, webhookPayID, webhookAmount), webhookEvtID, "sig")
	if err != nil {
		t.Fatalf("got %v, want nil (order not found must be swallowed)", err)
	}
}

func TestHandleWebhook_CaptureRepoError(t *testing.T) {
	dbErr := errors.New("mongo timeout")
	store := &fakeStore{byRzpErr: dbErr}
	svc := newWebhookSvc(t, store, &fakeEventStore{}, &fakeGateway{}, newFakeStock())
	err := svc.HandleWebhook(t.Context(), capturePayload(webhookRzpOID, webhookPayID, webhookAmount), webhookEvtID, "sig")
	if !errors.Is(err, dbErr) {
		t.Fatalf("got %v, want wrapped dbErr", err)
	}
}

func TestHandleWebhook_CaptureAmountMismatch(t *testing.T) {
	store := &fakeStore{byRzpOrder: webhookOrder(99900)} // total ≠ webhook amount 49900
	svc := newWebhookSvc(t, store, &fakeEventStore{}, &fakeGateway{}, newFakeStock())
	err := svc.HandleWebhook(t.Context(), capturePayload(webhookRzpOID, webhookPayID, webhookAmount), webhookEvtID, "sig")
	if err != nil {
		t.Fatalf("got %v, want nil (amount mismatch is a security log, not a retry)", err)
	}
}

func TestHandleWebhook_CaptureMarkPlacedError(t *testing.T) {
	store := &fakeStore{byRzpOrder: webhookOrder(webhookAmount), markPlacedErr: errors.New("mongo timeout")}
	svc := newWebhookSvc(t, store, &fakeEventStore{}, &fakeGateway{}, newFakeStock())
	err := svc.HandleWebhook(t.Context(), capturePayload(webhookRzpOID, webhookPayID, webhookAmount), webhookEvtID, "sig")
	if err == nil {
		t.Fatal("expected error from MarkPlaced, got nil")
	}
}

func TestHandleWebhook_CaptureSuccess(t *testing.T) {
	store := &fakeStore{byRzpOrder: webhookOrder(webhookAmount), markModified: true}
	clearer := &fakeCartClearer{}
	svc := newWebhookSvc(t, store, &fakeEventStore{}, &fakeGateway{}, newFakeStock(), clearer)
	err := svc.HandleWebhook(t.Context(), capturePayload(webhookRzpOID, webhookPayID, webhookAmount), webhookEvtID, "sig")
	if err != nil {
		t.Fatalf("got %v, want nil", err)
	}
	if !clearer.cleared {
		t.Error("cart must be cleared on successful capture")
	}
}

func TestHandleWebhook_CaptureAlreadyPlaced(t *testing.T) {
	// MarkPlaced returns (false, nil) — the callback already placed it.
	store := &fakeStore{byRzpOrder: webhookOrder(webhookAmount), markModified: false}
	clearer := &fakeCartClearer{}
	svc := newWebhookSvc(t, store, &fakeEventStore{}, &fakeGateway{}, newFakeStock(), clearer)
	err := svc.HandleWebhook(t.Context(), capturePayload(webhookRzpOID, webhookPayID, webhookAmount), webhookEvtID, "sig")
	if err != nil {
		t.Fatalf("got %v, want nil", err)
	}
	if clearer.cleared {
		t.Error("cart must NOT be cleared when modified=false")
	}
}

func TestHandleWebhook_CartClearError(t *testing.T) {
	store := &fakeStore{byRzpOrder: webhookOrder(webhookAmount), markModified: true}
	clearer := &fakeCartClearer{err: errors.New("cart db down")}
	svc := newWebhookSvc(t, store, &fakeEventStore{}, &fakeGateway{}, newFakeStock(), clearer)
	err := svc.HandleWebhook(t.Context(), capturePayload(webhookRzpOID, webhookPayID, webhookAmount), webhookEvtID, "sig")
	if err != nil {
		t.Fatalf("got %v, want nil (cart clear error must be swallowed)", err)
	}
	if !clearer.cleared {
		t.Error("Clear must have been attempted")
	}
}

func TestHandleWebhook_OrderPaidTreatedAsCapture(t *testing.T) {
	store := &fakeStore{byRzpOrder: webhookOrder(webhookAmount), markModified: true}
	svc := newWebhookSvc(t, store, &fakeEventStore{}, &fakeGateway{}, newFakeStock())
	paidBody := []byte(fmt.Sprintf(
		`{"event":"order.paid","payload":{"payment":{"entity":{"id":%q,"order_id":%q,"amount":%d}}}}`,
		webhookPayID, webhookRzpOID, webhookAmount,
	))
	err := svc.HandleWebhook(t.Context(), paidBody, "evt_paid", "sig")
	if err != nil {
		t.Fatalf("got %v, want nil (order.paid treated as capture)", err)
	}
}

func TestHandleWebhook_FailedNilPaymentWrapper(t *testing.T) {
	svc := newWebhookSvc(t, &fakeStore{}, &fakeEventStore{}, &fakeGateway{}, newFakeStock())
	err := svc.HandleWebhook(t.Context(), []byte(`{"event":"payment.failed","payload":{}}`), webhookEvtID, "sig")
	if err != nil {
		t.Fatalf("got %v, want nil (empty razorpayOrderId skips lookup)", err)
	}
}

func TestHandleWebhook_FailedOrderNotFound(t *testing.T) {
	store := &fakeStore{byRzpErr: order.ErrOrderNotFound}
	svc := newWebhookSvc(t, store, &fakeEventStore{}, &fakeGateway{}, newFakeStock())
	err := svc.HandleWebhook(t.Context(), failedPayload(webhookRzpOID, webhookPayID), webhookEvtID, "sig")
	if err != nil {
		t.Fatalf("got %v, want nil (order not found must be swallowed)", err)
	}
}

func TestHandleWebhook_FailedRepoError(t *testing.T) {
	dbErr := errors.New("mongo timeout")
	store := &fakeStore{byRzpErr: dbErr}
	svc := newWebhookSvc(t, store, &fakeEventStore{}, &fakeGateway{}, newFakeStock())
	err := svc.HandleWebhook(t.Context(), failedPayload(webhookRzpOID, webhookPayID), webhookEvtID, "sig")
	if !errors.Is(err, dbErr) {
		t.Fatalf("got %v, want wrapped dbErr", err)
	}
}

func TestHandleWebhook_FailedMarkFailedError(t *testing.T) {
	store := &fakeStore{byRzpOrder: webhookOrder(webhookAmount), markFailErr: errors.New("mongo timeout")}
	svc := newWebhookSvc(t, store, &fakeEventStore{}, &fakeGateway{}, newFakeStock())
	err := svc.HandleWebhook(t.Context(), failedPayload(webhookRzpOID, webhookPayID), webhookEvtID, "sig")
	if err == nil {
		t.Fatal("expected error from MarkPaymentFailed, got nil")
	}
}

func TestHandleWebhook_FailedSuccess(t *testing.T) {
	o := webhookOrder(webhookAmount)
	o.Lines = []order.Line{{ProductID: "tablets-120", Qty: 1}}
	store := &fakeStore{byRzpOrder: o, markFailModified: true}
	stock := newFakeStock()
	svc := newWebhookSvc(t, store, &fakeEventStore{}, &fakeGateway{}, stock)
	err := svc.HandleWebhook(t.Context(), failedPayload(webhookRzpOID, webhookPayID), webhookEvtID, "sig")
	if err != nil {
		t.Fatalf("got %v, want nil", err)
	}
	if stock.returned != 1 {
		t.Errorf("returned = %d, want 1 (stock must be released on payment failure)", stock.returned)
	}
}

/* ---------- fix-up tests: Processed flag semantics ---------- */

// TestHandleWebhook_DispatchErrorLeavesEventUnprocessed is the core correctness
// test for the bug fix: when dispatch fails the event must stay Processed:false
// so Razorpay's retry can reprocess it.
func TestHandleWebhook_DispatchErrorLeavesEventUnprocessed(t *testing.T) {
	store := &fakeStore{byRzpOrder: webhookOrder(webhookAmount), markPlacedErr: errors.New("mongo timeout")}
	events := &fakeEventStore{}
	svc := newWebhookSvc(t, store, events, &fakeGateway{}, newFakeStock())
	err := svc.HandleWebhook(t.Context(), capturePayload(webhookRzpOID, webhookPayID, webhookAmount), webhookEvtID, "sig")
	if err == nil {
		t.Fatal("expected error from dispatch, got nil")
	}
	if len(events.markedProcIDs) != 0 {
		t.Error("event must not be marked processed when dispatch fails")
	}
}

// TestHandleWebhook_RetryUnprocessedEventReprocesses covers a Razorpay retry
// of a delivery whose first attempt failed mid-dispatch (Processed:false).
// The retry must fall through to dispatch; the guarded transition is a no-op.
func TestHandleWebhook_RetryUnprocessedEventReprocesses(t *testing.T) {
	store := &fakeStore{byRzpOrder: webhookOrder(webhookAmount), markModified: true}
	events := &fakeEventStore{
		insertErr: order.ErrDuplicateEvent,
		getEvent:  order.PaymentEvent{Processed: false},
	}
	svc := newWebhookSvc(t, store, events, &fakeGateway{}, newFakeStock())
	err := svc.HandleWebhook(t.Context(), capturePayload(webhookRzpOID, webhookPayID, webhookAmount), webhookEvtID, "sig")
	if err != nil {
		t.Fatalf("got %v, want nil (retry of unprocessed event must succeed)", err)
	}
	if len(events.markedProcIDs) == 0 {
		t.Error("event must be marked processed after successful reprocessing")
	}
}

// TestHandleWebhook_HappyPathMarksEventProcessed verifies the insert/dispatch/mark
// sequence: event is inserted with Processed:false, dispatch runs, then
// MarkEventProcessed is called exactly once.
func TestHandleWebhook_HappyPathMarksEventProcessed(t *testing.T) {
	store := &fakeStore{byRzpOrder: webhookOrder(webhookAmount), markModified: true}
	events := &fakeEventStore{}
	svc := newWebhookSvc(t, store, events, &fakeGateway{}, newFakeStock())
	err := svc.HandleWebhook(t.Context(), capturePayload(webhookRzpOID, webhookPayID, webhookAmount), webhookEvtID, "sig")
	if err != nil {
		t.Fatalf("got %v, want nil", err)
	}
	if len(events.inserted) != 1 || events.inserted[0].Processed {
		t.Error("event must be inserted with Processed:false before dispatch")
	}
	if len(events.markedProcIDs) != 1 || events.markedProcIDs[0] != webhookEvtID {
		t.Errorf("markedProcIDs = %v, want [%s]", events.markedProcIDs, webhookEvtID)
	}
}

// TestHandleWebhook_GetEventErrorOnDuplicate covers the path where the duplicate
// insert triggers a GetEvent call that itself fails (e.g. DB flap). The handler
// must propagate the error so Razorpay retries.
func TestHandleWebhook_GetEventErrorOnDuplicate(t *testing.T) {
	dbErr := errors.New("mongo timeout")
	events := &fakeEventStore{
		insertErr: order.ErrDuplicateEvent,
		getErr:    dbErr,
	}
	svc := newWebhookSvc(t, &fakeStore{}, events, &fakeGateway{}, newFakeStock())
	err := svc.HandleWebhook(t.Context(), capturePayload(webhookRzpOID, webhookPayID, webhookAmount), webhookEvtID, "sig")
	if !errors.Is(err, dbErr) {
		t.Fatalf("got %v, want wrapped dbErr", err)
	}
}

// TestHandleWebhook_MarkProcessedErrorIsSwallowed verifies that a failure from
// MarkEventProcessed does not revert or fail the request — the transition already
// applied and Razorpay must receive 200.
func TestHandleWebhook_MarkProcessedErrorIsSwallowed(t *testing.T) {
	store := &fakeStore{byRzpOrder: webhookOrder(webhookAmount), markModified: true}
	events := &fakeEventStore{markProcErr: errors.New("mongo timeout")}
	svc := newWebhookSvc(t, store, events, &fakeGateway{}, newFakeStock())
	err := svc.HandleWebhook(t.Context(), capturePayload(webhookRzpOID, webhookPayID, webhookAmount), webhookEvtID, "sig")
	if err != nil {
		t.Fatalf("got %v, want nil (MarkEventProcessed error must be swallowed)", err)
	}
}

/* ------------------------------------------------------------ */

func TestHandleWebhook_FailedNotModified(t *testing.T) {
	o := webhookOrder(webhookAmount)
	o.Lines = []order.Line{{ProductID: "tablets-120", Qty: 1}}
	store := &fakeStore{byRzpOrder: o, markFailModified: false}
	stock := newFakeStock()
	svc := newWebhookSvc(t, store, &fakeEventStore{}, &fakeGateway{}, stock)
	err := svc.HandleWebhook(t.Context(), failedPayload(webhookRzpOID, webhookPayID), webhookEvtID, "sig")
	if err != nil {
		t.Fatalf("got %v, want nil", err)
	}
	if stock.returned != 0 {
		t.Errorf("returned = %d, want 0 (no stock release when guard rejected)", stock.returned)
	}
}

/* ------------------------------------------- confirmation email on capture */

// newSvcWithNotifier builds a service wired to a recording notifier.
func newSvcWithNotifier(t *testing.T, store order.Store, n order.Notifier) *order.Service {
	t.Helper()
	return order.NewService(order.ServiceConfig{
		Repo:          store,
		Cart:          &fakeLiner{},
		Auth:          &fakeAddr{addr: svcAddress},
		Catalogue:     newFakeStock(),
		Gateway:       &fakeGateway{},
		Events:        &fakeEventStore{},
		RazorpayKeyID: "rzp_test_KEY",
		Notifier:      n,
		Logger:        slog.New(slog.NewJSONHandler(io.Discard, nil)),
		Now:           func() time.Time { return fixedNow },
	})
}

// pendingOrderForCapture is an order awaiting payment, ready for the callback.
func pendingOrderForCapture() order.Order {
	return order.Order{
		OrderID: "EFF-483413",
		UserID:  testUser,
		Status:  order.StatusPendingPayment,
		Lines: []order.Line{{
			ProductID: "powder-100g", Name: "Pure Spirulina Powder - 100 g",
			Form: catalogue.FormPowder, UnitPrice: 20000, UnitMrp: 25000,
			Qty: 1, LineTotal: 20000,
		}},
		Totals: order.Totals{
			MRPTotal: 25000, Subtotal: 20000, Savings: 5000, Shipping: 5000, Total: 25000,
		},
		Payment:         order.NewPendingPayment("order_Pk9", 25000),
		ShippingAddress: svcAddress,
		CustomerPhone:   "9811111111",
		CreatedAt:       fixedNow,
		ExpiresAt:       fixedNow.Add(15 * time.Minute),
		UpdatedAt:       fixedNow,
	}
}

func TestConfirmPaymentSendsTheConfirmation(t *testing.T) {
	store := &fakeStore{byIDOrder: pendingOrderForCapture(), markModified: true}
	n := newRecordingNotifier()
	svc := newSvcWithNotifier(t, store, n)

	if _, err := svc.ConfirmPayment(t.Context(), testUser, "EFF-483413",
		"order_Pk9", "pay_Pk9", "sig"); err != nil {
		t.Fatalf("ConfirmPayment() error = %v", err)
	}

	n.waitForSend(t)
	if got := n.count(); got != 1 {
		t.Fatalf("%d confirmations sent, want 1", got)
	}

	n.mu.Lock()
	msg := n.sent[0]
	n.mu.Unlock()

	if msg.To != svcAddress.Email {
		t.Errorf("To = %q, want the order's email", msg.To)
	}
	// The email must describe the order as PLACED. Built from the pre-transition
	// read, it would tell a paying customer their order is still awaiting
	// payment.
	if !strings.Contains(msg.Subject, "EFF-483413") {
		t.Errorf("Subject = %q", msg.Subject)
	}
	if !strings.Contains(msg.TextBody, "confirmed") {
		t.Error("the body does not say the order is confirmed")
	}
}

// TestConfirmPaymentDoesNotSendWhenTheWebhookWonTheRace is the exactly-once
// pin. modified == false means another path already placed this order and
// already sent the email; sending again would double-mail the customer.
func TestConfirmPaymentDoesNotSendWhenTheWebhookWonTheRace(t *testing.T) {
	store := &fakeStore{
		byIDOrder:    pendingOrderForCapture(),
		markModified: false, // the webhook got there first
	}
	n := newRecordingNotifier()
	svc := newSvcWithNotifier(t, store, n)

	if _, err := svc.ConfirmPayment(t.Context(), testUser, "EFF-483413",
		"order_Pk9", "pay_Pk9", "sig"); err != nil {
		t.Fatalf("ConfirmPayment() error = %v", err)
	}

	// Nothing to wait for; give any stray goroutine a moment to misbehave.
	time.Sleep(150 * time.Millisecond)
	if got := n.count(); got != 0 {
		t.Errorf("%d confirmations sent, want 0 — the other path already mailed", got)
	}
}

// TestConfirmPaymentSucceedsWhenMailFails is the important one. The money has
// already moved and the order is already placed; a broken SMTP host must not
// turn that into a failed request.
func TestConfirmPaymentSucceedsWhenMailFails(t *testing.T) {
	store := &fakeStore{byIDOrder: pendingOrderForCapture(), markModified: true}
	n := newRecordingNotifier()
	n.err = errors.New("smtp: connection refused")
	svc := newSvcWithNotifier(t, store, n)

	got, err := svc.ConfirmPayment(t.Context(), testUser, "EFF-483413",
		"order_Pk9", "pay_Pk9", "sig")
	if err != nil {
		t.Fatalf("ConfirmPayment() failed because mail failed: %v", err)
	}
	if got.Status != order.StatusPlaced {
		t.Errorf("status = %q, want placed", got.Status)
	}
	n.waitForSend(t) // it was attempted
}

func TestConfirmPaymentWithoutANotifier(t *testing.T) {
	// Nil notifier is the unconfigured case and must not panic.
	store := &fakeStore{byIDOrder: pendingOrderForCapture(), markModified: true}
	svc := newSvcWithNotifier(t, store, nil)

	if _, err := svc.ConfirmPayment(t.Context(), testUser, "EFF-483413",
		"order_Pk9", "pay_Pk9", "sig"); err != nil {
		t.Fatalf("ConfirmPayment() error = %v", err)
	}
}

func TestConfirmPaymentWithNoEmailOnTheOrder(t *testing.T) {
	// Nothing to send to. The order must still place cleanly.
	o := pendingOrderForCapture()
	o.ShippingAddress.Email = ""
	store := &fakeStore{byIDOrder: o, markModified: true}
	n := newRecordingNotifier()
	svc := newSvcWithNotifier(t, store, n)

	got, err := svc.ConfirmPayment(t.Context(), testUser, "EFF-483413",
		"order_Pk9", "pay_Pk9", "sig")
	if err != nil {
		t.Fatalf("ConfirmPayment() error = %v", err)
	}
	if got.Status != order.StatusPlaced {
		t.Errorf("status = %q, want placed", got.Status)
	}
	time.Sleep(150 * time.Millisecond)
	if n.count() != 0 {
		t.Error("a confirmation was sent with no recipient")
	}
}
