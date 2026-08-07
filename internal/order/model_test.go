package order_test

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/enerzia/enerzia-be/internal/auth"
	"github.com/enerzia/enerzia-be/internal/catalogue"
	"github.com/enerzia/enerzia-be/internal/order"
)

// ─── Status ──────────────────────────────────────────────────────────────────

func TestStatusValid(t *testing.T) {
	tests := []struct {
		in   order.Status
		want bool
	}{
		{order.StatusPendingPayment, true},
		{order.StatusPlaced, true},
		{order.StatusPaymentFailed, true},
		{order.StatusExpired, true},
		{order.StatusPacked, true},
		{order.StatusShipped, true},
		{order.StatusDelivered, true},
		{order.StatusCancelled, true},
		{"pending", false},
		{"cod", false},
		{"PLACED", false}, // case-sensitive, like the roadmap literals
		{"", false},
	}
	for _, tt := range tests {
		t.Run(string(tt.in), func(t *testing.T) {
			if got := tt.in.Valid(); got != tt.want {
				t.Errorf("Status(%q).Valid() = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// ─── PaymentStatus ───────────────────────────────────────────────────────────

func TestPaymentStatusValid(t *testing.T) {
	tests := []struct {
		in   order.PaymentStatus
		want bool
	}{
		{order.PaymentStatusCreated, true},
		{order.PaymentStatusAuthorized, true},
		{order.PaymentStatusCaptured, true},
		{order.PaymentStatusFailed, true},
		{order.PaymentStatusRefunded, true},
		{"pending", false},
		{"CAPTURED", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(string(tt.in), func(t *testing.T) {
			if got := tt.in.Valid(); got != tt.want {
				t.Errorf("PaymentStatus(%q).Valid() = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// ─── PaymentMethod ───────────────────────────────────────────────────────────

func TestPaymentMethodValidAndLabel(t *testing.T) {
	// Labels are stored on the order so a receipt renders correctly even if
	// this copy changes later (schema.md §orders).
	tests := []struct {
		method    order.PaymentMethod
		wantValid bool
		wantLabel string
	}{
		{order.PaymentUPI, true, "UPI"},
		{order.PaymentCard, true, "Card"},
		{order.PaymentNetbanking, true, "Netbanking"},
		{order.PaymentWallet, true, "Wallet"},
		{order.PaymentEMI, true, "EMI"},
		{order.PaymentPayLater, true, "Pay Later"},
		// COD was removed when payment moved to Razorpay.
		{"cod", false, ""},
		{"cash", false, ""},
		{"", false, ""},
	}
	for _, tt := range tests {
		t.Run(string(tt.method), func(t *testing.T) {
			if got := tt.method.Valid(); got != tt.wantValid {
				t.Errorf("Valid() = %v, want %v", got, tt.wantValid)
			}
			if got := tt.method.Label(); got != tt.wantLabel {
				t.Errorf("Label() = %q, want %q", got, tt.wantLabel)
			}
		})
	}
}

// ─── NewPendingPayment ───────────────────────────────────────────────────────

func TestNewPendingPayment(t *testing.T) {
	p := order.NewPendingPayment("order_PkX9aQ", 114000)

	if p.Provider != "razorpay" {
		t.Errorf("Provider = %q, want %q", p.Provider, "razorpay")
	}
	if p.Status != order.PaymentStatusCreated {
		t.Errorf("Status = %q, want %q", p.Status, order.PaymentStatusCreated)
	}
	if p.Amount != 114000 {
		t.Errorf("Amount = %d, want 114000", p.Amount)
	}
	if p.Currency != "INR" {
		t.Errorf("Currency = %q, want %q", p.Currency, "INR")
	}
	if p.RazorpayOrderID != "order_PkX9aQ" {
		t.Errorf("RazorpayOrderID = %q, want %q", p.RazorpayOrderID, "order_PkX9aQ")
	}
	// Method and all detail fields must be empty in the pending state.
	if p.Method != "" || p.Label != "" || p.Last4 != "" || p.Network != "" ||
		p.Bank != "" || p.Wallet != "" || p.VPA != "" {
		t.Error("NewPendingPayment() set a method or detail field; those come from Razorpay later")
	}
	if p.CapturedAt != nil {
		t.Error("NewPendingPayment() set CapturedAt; it must be nil until capture")
	}
	if err := p.Validate(); err != nil {
		t.Errorf("NewPendingPayment().Validate() = %v, want nil", err)
	}
}

// ─── Payment.Validate ────────────────────────────────────────────────────────

// validPendingPayment returns a payment in the initial (no-method) state.
func validPendingPayment() order.Payment {
	return order.NewPendingPayment("order_PkX9aQ", 114000)
}

// validCapturedPayment returns a fully resolved payment for a placed order.
func validCapturedPayment() order.Payment {
	now := time.Now()
	return order.Payment{
		Provider:          "razorpay",
		Status:            order.PaymentStatusCaptured,
		Amount:            114000,
		Currency:          "INR",
		RazorpayOrderID:   "order_PkX9aQ",
		RazorpayPaymentID: "pay_PkXB7z",
		Method:            order.PaymentUPI,
		Label:             "UPI",
		VPA:               "ananya@okaxis",
		CapturedAt:        &now,
	}
}

func TestPaymentValidate(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		payment order.Payment
		wantErr bool
	}{
		// ── Pending (no method) ─────────────────────────────────────────────
		{"pending: valid", validPendingPayment(), false},

		// Gateway-field failures (checked before method)
		{"bad provider", mutatePayment(validPendingPayment(), func(p *order.Payment) { p.Provider = "stripe" }), true},
		{"unknown payment status", mutatePayment(validPendingPayment(), func(p *order.Payment) { p.Status = "waiting" }), true},
		{"zero amount", mutatePayment(validPendingPayment(), func(p *order.Payment) { p.Amount = 0 }), true},
		{"negative amount", mutatePayment(validPendingPayment(), func(p *order.Payment) { p.Amount = -1 }), true},
		{"wrong currency", mutatePayment(validPendingPayment(), func(p *order.Payment) { p.Currency = "USD" }), true},
		// A created payment with no RazorpayOrderID is valid: it represents the
		// insert-before-Razorpay state where the link is not yet established.
		{"created: empty razorpay order id is ok", mutatePayment(validPendingPayment(), func(p *order.Payment) { p.RazorpayOrderID = "" }), false},
		// Any other status must have a RazorpayOrderID.
		{"authorized: empty razorpay order id", mutatePayment(validPendingPayment(), func(p *order.Payment) {
			p.Status = order.PaymentStatusAuthorized
			p.RazorpayOrderID = ""
		}), true},

		// Detail fields on a pending (no-method) payment are all rejected.
		{"pending: label set", mutatePayment(validPendingPayment(), func(p *order.Payment) { p.Label = "UPI" }), true},
		{"pending: last4 set", mutatePayment(validPendingPayment(), func(p *order.Payment) { p.Last4 = "1111" }), true},
		{"pending: network set", mutatePayment(validPendingPayment(), func(p *order.Payment) { p.Network = "Visa" }), true},
		{"pending: bank set", mutatePayment(validPendingPayment(), func(p *order.Payment) { p.Bank = "HDFC" }), true},
		{"pending: wallet set", mutatePayment(validPendingPayment(), func(p *order.Payment) { p.Wallet = "Paytm" }), true},
		{"pending: vpa set", mutatePayment(validPendingPayment(), func(p *order.Payment) { p.VPA = "ananya@okaxis" }), true},

		// ── Method-set payments ─────────────────────────────────────────────
		{"captured: valid upi", validCapturedPayment(), false},

		{"method-set: unknown method", mutatePayment(validCapturedPayment(), func(p *order.Payment) {
			p.Method = "cod"
			p.Label = ""
		}), true},
		{"method-set: label mismatch", mutatePayment(validCapturedPayment(), func(p *order.Payment) {
			p.Label = "Card"
		}), true},

		// ── Card ────────────────────────────────────────────────────────────
		{"card: valid with last4 and network", order.Payment{
			Provider: "razorpay", Status: order.PaymentStatusCaptured,
			Amount: 114000, Currency: "INR", RazorpayOrderID: "order_x",
			Method: order.PaymentCard, Label: "Card",
			Last4: "4242", Network: "Visa",
			CapturedAt: &now,
		}, false},
		{"card: valid without last4", order.Payment{
			Provider: "razorpay", Status: order.PaymentStatusCaptured,
			Amount: 114000, Currency: "INR", RazorpayOrderID: "order_x",
			Method: order.PaymentCard, Label: "Card",
			CapturedAt: &now,
		}, false},
		{"card: stray vpa", order.Payment{
			Provider: "razorpay", Status: order.PaymentStatusCaptured,
			Amount: 114000, Currency: "INR", RazorpayOrderID: "order_x",
			Method: order.PaymentCard, Label: "Card", VPA: "a@b",
		}, true},
		{"card: stray bank", order.Payment{
			Provider: "razorpay", Status: order.PaymentStatusCaptured,
			Amount: 114000, Currency: "INR", RazorpayOrderID: "order_x",
			Method: order.PaymentCard, Label: "Card", Bank: "HDFC",
		}, true},
		{"card: stray wallet", order.Payment{
			Provider: "razorpay", Status: order.PaymentStatusCaptured,
			Amount: 114000, Currency: "INR", RazorpayOrderID: "order_x",
			Method: order.PaymentCard, Label: "Card", Wallet: "Paytm",
		}, true},
		{"card: last4 too short", order.Payment{
			Provider: "razorpay", Status: order.PaymentStatusCaptured,
			Amount: 114000, Currency: "INR", RazorpayOrderID: "order_x",
			Method: order.PaymentCard, Label: "Card", Last4: "111",
		}, true},
		{"card: last4 non-digit", order.Payment{
			Provider: "razorpay", Status: order.PaymentStatusCaptured,
			Amount: 114000, Currency: "INR", RazorpayOrderID: "order_x",
			Method: order.PaymentCard, Label: "Card", Last4: "11a1",
		}, true},

		// ── UPI ─────────────────────────────────────────────────────────────
		{"upi: valid with vpa", validCapturedPayment(), false},
		{"upi: valid without vpa", order.Payment{
			Provider: "razorpay", Status: order.PaymentStatusCaptured,
			Amount: 114000, Currency: "INR", RazorpayOrderID: "order_x",
			Method: order.PaymentUPI, Label: "UPI",
			CapturedAt: &now,
		}, false},
		{"upi: stray last4", mutatePayment(validCapturedPayment(), func(p *order.Payment) { p.Last4 = "1111" }), true},
		{"upi: stray network", mutatePayment(validCapturedPayment(), func(p *order.Payment) { p.Network = "Visa" }), true},
		{"upi: stray bank", mutatePayment(validCapturedPayment(), func(p *order.Payment) { p.Bank = "HDFC" }), true},
		{"upi: stray wallet", mutatePayment(validCapturedPayment(), func(p *order.Payment) { p.Wallet = "Paytm" }), true},

		// ── Netbanking ──────────────────────────────────────────────────────
		{"netbanking: valid with bank", order.Payment{
			Provider: "razorpay", Status: order.PaymentStatusCaptured,
			Amount: 114000, Currency: "INR", RazorpayOrderID: "order_x",
			Method: order.PaymentNetbanking, Label: "Netbanking", Bank: "HDFC",
			CapturedAt: &now,
		}, false},
		{"netbanking: stray last4", order.Payment{
			Provider: "razorpay", Status: order.PaymentStatusCaptured,
			Amount: 114000, Currency: "INR", RazorpayOrderID: "order_x",
			Method: order.PaymentNetbanking, Label: "Netbanking", Last4: "1111",
		}, true},
		{"netbanking: stray vpa", order.Payment{
			Provider: "razorpay", Status: order.PaymentStatusCaptured,
			Amount: 114000, Currency: "INR", RazorpayOrderID: "order_x",
			Method: order.PaymentNetbanking, Label: "Netbanking", VPA: "a@b",
		}, true},
		{"netbanking: stray wallet", order.Payment{
			Provider: "razorpay", Status: order.PaymentStatusCaptured,
			Amount: 114000, Currency: "INR", RazorpayOrderID: "order_x",
			Method: order.PaymentNetbanking, Label: "Netbanking", Wallet: "Paytm",
		}, true},

		// ── Wallet ──────────────────────────────────────────────────────────
		{"wallet: valid", order.Payment{
			Provider: "razorpay", Status: order.PaymentStatusCaptured,
			Amount: 114000, Currency: "INR", RazorpayOrderID: "order_x",
			Method: order.PaymentWallet, Label: "Wallet", Wallet: "Paytm",
			CapturedAt: &now,
		}, false},
		{"wallet: stray last4", order.Payment{
			Provider: "razorpay", Status: order.PaymentStatusCaptured,
			Amount: 114000, Currency: "INR", RazorpayOrderID: "order_x",
			Method: order.PaymentWallet, Label: "Wallet", Last4: "1111",
		}, true},
		{"wallet: stray vpa", order.Payment{
			Provider: "razorpay", Status: order.PaymentStatusCaptured,
			Amount: 114000, Currency: "INR", RazorpayOrderID: "order_x",
			Method: order.PaymentWallet, Label: "Wallet", VPA: "a@b",
		}, true},
		{"wallet: stray bank", order.Payment{
			Provider: "razorpay", Status: order.PaymentStatusCaptured,
			Amount: 114000, Currency: "INR", RazorpayOrderID: "order_x",
			Method: order.PaymentWallet, Label: "Wallet", Bank: "HDFC",
		}, true},

		// ── EMI ─────────────────────────────────────────────────────────────
		{"emi: valid", order.Payment{
			Provider: "razorpay", Status: order.PaymentStatusCaptured,
			Amount: 114000, Currency: "INR", RazorpayOrderID: "order_x",
			Method: order.PaymentEMI, Label: "EMI",
			CapturedAt: &now,
		}, false},
		{"emi: stray last4", order.Payment{
			Provider: "razorpay", Status: order.PaymentStatusCaptured,
			Amount: 114000, Currency: "INR", RazorpayOrderID: "order_x",
			Method: order.PaymentEMI, Label: "EMI", Last4: "1111",
		}, true},
		{"emi: stray vpa", order.Payment{
			Provider: "razorpay", Status: order.PaymentStatusCaptured,
			Amount: 114000, Currency: "INR", RazorpayOrderID: "order_x",
			Method: order.PaymentEMI, Label: "EMI", VPA: "a@b",
		}, true},

		// ── PayLater ────────────────────────────────────────────────────────
		{"paylater: valid", order.Payment{
			Provider: "razorpay", Status: order.PaymentStatusCaptured,
			Amount: 114000, Currency: "INR", RazorpayOrderID: "order_x",
			Method: order.PaymentPayLater, Label: "Pay Later",
			CapturedAt: &now,
		}, false},
		{"paylater: stray bank", order.Payment{
			Provider: "razorpay", Status: order.PaymentStatusCaptured,
			Amount: 114000, Currency: "INR", RazorpayOrderID: "order_x",
			Method: order.PaymentPayLater, Label: "Pay Later", Bank: "HDFC",
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.payment.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// mutatePayment copies p, applies fn, and returns the mutated copy.
func mutatePayment(p order.Payment, fn func(*order.Payment)) order.Payment {
	fn(&p)
	return p
}

// ─── Line.Validate ───────────────────────────────────────────────────────────

// validLine is a well-formed snapshot line the failure cases mutate away from.
func validLine() order.Line {
	return order.Line{
		ProductID: "tablets-120",
		Name:      "Spirulina Tablets 500 mg — 120 tabs",
		Form:      catalogue.FormTablets,
		Grad:      "radial-gradient(...)",
		UnitPrice: 38000,
		UnitMrp:   47000,
		Qty:       3,
		LineTotal: 114000,
	}
}

func TestLineValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*order.Line)
		wantErr bool
	}{
		{"valid", func(*order.Line) {}, false},
		{"blank product id", func(l *order.Line) { l.ProductID = "" }, true},
		{"blank name", func(l *order.Line) { l.Name = "" }, true},
		{"unknown form", func(l *order.Line) { l.Form = "Liquid" }, true},
		{"zero qty", func(l *order.Line) { l.Qty = 0 }, true},
		{"negative qty", func(l *order.Line) { l.Qty = -1 }, true},
		{"zero unit price", func(l *order.Line) { l.UnitPrice = 0 }, true},
		{"zero unit mrp", func(l *order.Line) { l.UnitMrp = 0 }, true},
		{"price above mrp", func(l *order.Line) { l.UnitPrice = 50000 }, true},
		{"line total mismatch", func(l *order.Line) { l.LineTotal = 999 }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := validLine()
			tt.mutate(&l)
			if err := l.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ─── Totals.Validate ─────────────────────────────────────────────────────────

func validTotals() order.Totals {
	return order.Totals{MRPTotal: 141000, Subtotal: 114000, Savings: 27000, Shipping: 0, Total: 114000}
}

func TestTotalsValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*order.Totals)
		wantErr bool
	}{
		{"valid", func(*order.Totals) {}, false},
		{"valid with shipping", func(tot *order.Totals) {
			*tot = order.Totals{MRPTotal: 30000, Subtotal: 20000, Savings: 10000, Shipping: 4900, Total: 24900}
		}, false},
		{"zero subtotal", func(tot *order.Totals) { tot.Subtotal = 0 }, true},
		{"negative subtotal", func(tot *order.Totals) { tot.Subtotal = -1 }, true},
		{"mrp below subtotal", func(tot *order.Totals) { tot.MRPTotal = 100000 }, true},
		{"negative shipping", func(tot *order.Totals) { tot.Shipping = -1 }, true},
		{"wrong savings", func(tot *order.Totals) { tot.Savings = 1 }, true},
		{"wrong total", func(tot *order.Totals) { tot.Total = 1 }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tot := validTotals()
			tt.mutate(&tot)
			if err := tot.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ─── Order.Validate ──────────────────────────────────────────────────────────

func validAddress() auth.Address {
	return auth.Address{
		Name:  "Ananya Sharma",
		Email: "ananya@example.com",
		Line1: "12, Anand Residency, MG Road",
		City:  "Pune",
		State: "Maharashtra",
		Pin:   "411001",
	}
}

// validPendingOrder returns a well-formed pending_payment order. PlacedAt is
// nil and the payment has no method — that is the correct state before Razorpay
// confirms capture.
func validPendingOrder() order.Order {
	now := time.Now()
	return order.Order{
		OrderID:         "EFF-483413",
		UserID:          bson.NewObjectID(),
		Status:          order.StatusPendingPayment,
		Lines:           []order.Line{validLine()},
		Totals:          validTotals(),
		Payment:         validPendingPayment(),
		ShippingAddress: validAddress(),
		CreatedAt:       now,
		ExpiresAt:       now.Add(15 * time.Minute),
	}
}

// validPlacedOrder returns a well-formed placed order with PlacedAt and a
// captured payment set. This is the state after Razorpay confirms payment.
func validPlacedOrder() order.Order {
	now := time.Now()
	o := validPendingOrder()
	o.Status = order.StatusPlaced
	o.Payment = validCapturedPayment()
	o.PlacedAt = &now
	return o
}

func TestOrderValidate(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		mutate  func(*order.Order)
		base    func() order.Order
		wantErr bool
	}{
		// ── Pending order ───────────────────────────────────────────────────
		{"pending: valid", func(*order.Order) {}, validPendingOrder, false},
		{"pending: nil PlacedAt is fine", func(o *order.Order) { o.PlacedAt = nil }, validPendingOrder, false},
		{"pending: payment_failed is valid", func(o *order.Order) { o.Status = order.StatusPaymentFailed }, validPendingOrder, false},
		{"pending: expired is valid", func(o *order.Order) { o.Status = order.StatusExpired }, validPendingOrder, false},

		// ── Placed order ────────────────────────────────────────────────────
		{"placed: valid", func(*order.Order) {}, validPlacedOrder, false},
		{"placed: nil PlacedAt", func(o *order.Order) { o.PlacedAt = nil }, validPlacedOrder, true},
		{"placed: empty payment method", func(o *order.Order) {
			o.Payment = order.NewPendingPayment("order_x", 114000)
		}, validPlacedOrder, true},
		{"placed: nil capturedAt", func(o *order.Order) { o.Payment.CapturedAt = nil }, validPlacedOrder, true},

		// packed/shipped/delivered/cancelled also require placement evidence.
		{"packed: valid", func(o *order.Order) { o.Status = order.StatusPacked }, validPlacedOrder, false},
		{"shipped: valid", func(o *order.Order) { o.Status = order.StatusShipped }, validPlacedOrder, false},
		{"delivered: valid", func(o *order.Order) { o.Status = order.StatusDelivered }, validPlacedOrder, false},
		{"cancelled: valid", func(o *order.Order) { o.Status = order.StatusCancelled }, validPlacedOrder, false},
		{"cancelled: missing placedAt", func(o *order.Order) {
			o.Status = order.StatusCancelled
			o.PlacedAt = nil
		}, validPlacedOrder, true},

		// ── Always-checked fields ────────────────────────────────────────────
		{"bad order id", func(o *order.Order) { o.OrderID = "EFF-42" }, validPendingOrder, true},
		{"zero user id", func(o *order.Order) { o.UserID = bson.ObjectID{} }, validPendingOrder, true},
		{"unknown status", func(o *order.Order) { o.Status = "waiting" }, validPendingOrder, true},
		{"no lines", func(o *order.Order) { o.Lines = nil }, validPendingOrder, true},
		{"bad line propagates", func(o *order.Order) {
			o.Lines = []order.Line{{ProductID: "x"}}
		}, validPendingOrder, true},
		{"bad totals propagates", func(o *order.Order) { o.Totals.Total = 1 }, validPendingOrder, true},
		{"bad payment propagates", func(o *order.Order) {
			o.Payment = order.Payment{Provider: "stripe", Status: order.PaymentStatusCreated, Amount: 1, Currency: "INR", RazorpayOrderID: "x"}
		}, validPendingOrder, true},

		// ── Detail field on wrong method ─────────────────────────────────────
		{"placed: upi with stray last4", func(o *order.Order) {
			o.Payment.Last4 = "1234"
		}, validPlacedOrder, true},
		{"placed: card with stray vpa", func(o *order.Order) {
			o.Payment.Method = order.PaymentCard
			o.Payment.Label = "Card"
			o.Payment.VPA = "a@b"
			o.Payment.CapturedAt = &now
		}, validPlacedOrder, true},

		// ── Payment.Amount vs Totals.Total (defence in depth, schema.md §Razorpay)
		// The check applies to every status — a pending order with the wrong
		// amount is as wrong as a placed one.
		{"pending: amount below total", func(o *order.Order) {
			o.Payment.Amount = 100
		}, validPendingOrder, true},
		{"pending: amount above total", func(o *order.Order) {
			o.Payment.Amount = 999999
		}, validPendingOrder, true},
		{"placed: amount below total", func(o *order.Order) {
			o.Payment.Amount = 100
		}, validPlacedOrder, true},
		{"placed: amount above total", func(o *order.Order) {
			o.Payment.Amount = 999999
		}, validPlacedOrder, true},

		// ── Payment status must reflect money having moved for placed+ orders ──
		// captured is the normal case; refunded is allowed (money moved, then
		// was returned — the order itself still happened).
		{"placed: payment status captured", func(*order.Order) {}, validPlacedOrder, false},
		{"placed: payment status refunded", func(o *order.Order) {
			o.Payment.Status = order.PaymentStatusRefunded
		}, validPlacedOrder, false},
		{"placed: payment status failed", func(o *order.Order) {
			o.Payment.Status = order.PaymentStatusFailed
		}, validPlacedOrder, true},
		{"placed: payment status created", func(o *order.Order) {
			o.Payment.Status = order.PaymentStatusCreated
		}, validPlacedOrder, true},
		{"placed: payment status authorized", func(o *order.Order) {
			o.Payment.Status = order.PaymentStatusAuthorized
		}, validPlacedOrder, true},
		// A pending_payment order is not subject to the payment-status rule —
		// "created" is exactly right for a reservation that hasn't been paid yet.
		{"pending: payment status created is fine", func(*order.Order) {}, validPendingOrder, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := tt.base()
			tt.mutate(&o)
			if err := o.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestOrderItemCount(t *testing.T) {
	o := order.Order{Lines: []order.Line{{Qty: 3}, {Qty: 2}}}
	if got := o.ItemCount(); got != 5 {
		t.Errorf("ItemCount() = %d, want 5", got)
	}
	if got := (order.Order{}).ItemCount(); got != 0 {
		t.Errorf("ItemCount() on empty order = %d, want 0", got)
	}
}
