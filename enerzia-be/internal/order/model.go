// Package order owns a placed order: what was bought, what it cost, how it was
// paid for, and where it ships.
//
// An order exists before it is paid for (schema.md decision 6). It starts in
// pending_payment holding a stock reservation; a verified Razorpay callback or
// webhook moves it to placed. Lines, totals and the shipping address are frozen
// at creation time and never re-read from the live catalogue or address book
// (schema.md decision 4).
//
// Everything here is pure: no I/O, no MongoDB driver calls, no net/http. Money
// is int64 paise everywhere, never float and never rupees.
package order

import (
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/enerzia/enerzia-be/internal/auth"
	"github.com/enerzia/enerzia-be/internal/catalogue"
)

// currencyINR is the only payment currency this shop accepts.
const currencyINR = "INR"

// providerRazorpay is the only payment provider.
const providerRazorpay = "razorpay"

// ─── Order status ────────────────────────────────────────────────────────────

// Status is the fulfilment stage of an order (schema.md §orders lifecycle).
type Status string

// The full lifecycle. Only Razorpay moves a status: it records what happened to
// the money. What happened to the parcel is the separate Fulfilment field
// below.
const (
	// StatusPendingPayment holds a stock reservation while Razorpay collects
	// the payment. The order is real but the money has not moved yet.
	StatusPendingPayment Status = "pending_payment"
	// StatusPlaced means Razorpay confirmed the capture. The shopper would
	// call this "ordered".
	StatusPlaced Status = "placed"
	// StatusPaymentFailed means Razorpay reported a declined or failed
	// attempt. The stock reservation is released.
	StatusPaymentFailed Status = "payment_failed"
	// StatusExpired means the 15-minute reservation elapsed without payment.
	// The stock reservation is released.
	StatusExpired Status = "expired"

	// StatusPacked, StatusShipped, StatusDelivered and StatusCancelled are
	// valid stored values that nothing produces. They predate the fulfilment
	// split (schema.md §orders, 2026-08-17), which moved parcel progress to its
	// own field so an operator could not rewrite an order's payment state by
	// marking a box packed. They are kept so stored documents and the contract
	// stay in step, and because ListForUser still includes them — a cancelled
	// order was paid for, and hiding it would erase a purchase the shopper
	// remembers making. Removing them is task 11.9.
	StatusPacked    Status = "packed"
	StatusShipped   Status = "shipped"
	StatusDelivered Status = "delivered"
	StatusCancelled Status = "cancelled"
)

// Valid reports whether s is a status the system recognises.
func (s Status) Valid() bool {
	switch s {
	case StatusPendingPayment, StatusPlaced, StatusPaymentFailed, StatusExpired,
		StatusPacked, StatusShipped, StatusDelivered, StatusCancelled:
		return true
	default:
		return false
	}
}

// Label is the display string for a status, stored nowhere and computed at
// response time. An unrecognised status yields "" rather than a guess,
// following PaymentMethod.Label.
func (s Status) Label() string {
	switch s {
	case StatusPendingPayment:
		return "Payment pending"
	case StatusPlaced:
		return "Placed"
	case StatusPaymentFailed:
		return "Payment failed"
	case StatusExpired:
		return "Expired"
	case StatusPacked:
		return "Packed"
	case StatusShipped:
		return "Shipped"
	case StatusDelivered:
		return "Delivered"
	case StatusCancelled:
		return "Cancelled"
	default:
		return ""
	}
}

// ─── Fulfilment ──────────────────────────────────────────────────────────────

// Fulfilment is what a person did with the parcel, tracked separately from
// Status so the two cannot interfere (schema.md §orders, roadmap.md §Admin
// orders). Status records what Razorpay did with the money; a single field
// carrying both would mean an operator marking a box packed had rewritten what
// the order says about payment.
//
// Only the admin order book writes it. No shopper-facing endpoint reads it —
// an order stays "Confirmed" to its buyer through every step here, and mapping
// these states into the account area is a later decision.
type Fulfilment string

// The progression, in order. There is no delivered value because nothing tells
// us a parcel arrived, and no cancelled value because there is no cancellation
// and no refund.
const (
	// FulfilmentNone is the zero value: paid for, nothing done to it yet. It is
	// the absence of the field on the document, not a stored string — which is
	// also why the split needed no migration.
	FulfilmentNone Fulfilment = ""
	// FulfilmentPacked is boxed, label printed, waiting for the courier.
	FulfilmentPacked Fulfilment = "packed"
	// FulfilmentInTransit is handed over and moving.
	FulfilmentInTransit Fulfilment = "in_transit"
	// FulfilmentShipped is terminal: the parcel is out of our hands.
	FulfilmentShipped Fulfilment = "shipped"
)

// Valid reports whether f is a fulfilment state the system recognises.
// FulfilmentNone is valid: "not started" is a state, not a missing value.
func (f Fulfilment) Valid() bool {
	switch f {
	case FulfilmentNone, FulfilmentPacked, FulfilmentInTransit, FulfilmentShipped:
		return true
	default:
		return false
	}
}

// Label is the display string for a fulfilment state, computed at response
// time and stored nowhere. An unrecognised value yields "" rather than a guess.
func (f Fulfilment) Label() string {
	switch f {
	case FulfilmentNone:
		return "Not started"
	case FulfilmentPacked:
		return "Processed"
	case FulfilmentInTransit:
		return "Transit"
	case FulfilmentShipped:
		return "Shipped"
	default:
		return ""
	}
}

// fulfilmentOrder is the linear progression an operator drives by hand.
// CanFulfil is derived from it rather than from a hand-written pair list, so a
// new stage is added by inserting one element here and cannot exist in the
// sequence without also being permitted by the guard.
var fulfilmentOrder = []Fulfilment{
	FulfilmentNone,
	FulfilmentPacked,
	FulfilmentInTransit,
	FulfilmentShipped,
}

// CanFulfil reports whether an operator may move an order from one fulfilment
// state to another (roadmap.md §Admin orders).
//
// The progression is strictly linear and forward by exactly one step.
// Backwards moves and skips are both refused: jumping to shipped would skip the
// step at which a label gets printed, and there is no legitimate reason to
// un-ship a parcel that has already gone. A self-move is refused too — a
// no-op that answered 200 would tell an operator a parcel moved when nothing
// did.
func CanFulfil(from, to Fulfilment) bool {
	next, ok := NextFulfilment(from)
	return ok && next == to
}

// NextFulfilment returns the only state an order may move to from f, and
// whether one exists. It is false for FulfilmentShipped, which is terminal, and
// for any value outside the sequence.
//
// Exported because the 409 an operator reads is far more useful when it names
// what they *can* do than when it only says no.
func NextFulfilment(f Fulfilment) (Fulfilment, bool) {
	for i, cur := range fulfilmentOrder {
		if cur == f {
			if i+1 < len(fulfilmentOrder) {
				return fulfilmentOrder[i+1], true
			}
			return FulfilmentNone, false
		}
	}
	return FulfilmentNone, false
}

// ─── Payment status ──────────────────────────────────────────────────────────

// PaymentStatus is the Razorpay-side state of the payment sub-document.
type PaymentStatus string

// Payment lifecycle per Razorpay's payment object.
const (
	PaymentStatusCreated    PaymentStatus = "created"
	PaymentStatusAuthorized PaymentStatus = "authorized"
	PaymentStatusCaptured   PaymentStatus = "captured"
	PaymentStatusFailed     PaymentStatus = "failed"
	PaymentStatusRefunded   PaymentStatus = "refunded"
)

// Valid reports whether s is a payment status the system recognises.
func (s PaymentStatus) Valid() bool {
	switch s {
	case PaymentStatusCreated, PaymentStatusAuthorized, PaymentStatusCaptured,
		PaymentStatusFailed, PaymentStatusRefunded:
		return true
	default:
		return false
	}
}

// ─── Payment method ──────────────────────────────────────────────────────────

// PaymentMethod is what Razorpay reports after the shopper pays. The method is
// unknown until Razorpay confirms it, so Method is empty on a pending_payment
// order. There is no cash-on-delivery: the shopper picks inside Razorpay's
// modal, and no payment details ever reach this server (schema.md §Razorpay).
type PaymentMethod string

// Payment methods Razorpay may report.
const (
	PaymentUPI        PaymentMethod = "upi"
	PaymentCard       PaymentMethod = "card"
	PaymentNetbanking PaymentMethod = "netbanking"
	PaymentWallet     PaymentMethod = "wallet"
	PaymentEMI        PaymentMethod = "emi"
	PaymentPayLater   PaymentMethod = "paylater"
)

// Valid reports whether m is a Razorpay-reported method the system recognises.
func (m PaymentMethod) Valid() bool {
	switch m {
	case PaymentUPI, PaymentCard, PaymentNetbanking, PaymentWallet, PaymentEMI, PaymentPayLater:
		return true
	default:
		return false
	}
}

// Label is the display string stored alongside the method so a receipt renders
// correctly even if this copy changes later (schema.md §orders). An unknown
// method yields "" rather than a guess.
func (m PaymentMethod) Label() string {
	switch m {
	case PaymentUPI:
		return "UPI"
	case PaymentCard:
		return "Card"
	case PaymentNetbanking:
		return "Netbanking"
	case PaymentWallet:
		return "Wallet"
	case PaymentEMI:
		return "EMI"
	case PaymentPayLater:
		return "Pay Later"
	default:
		return ""
	}
}

// ─── Payment sub-document ────────────────────────────────────────────────────

// PaymentFailure carries Razorpay's error detail when an attempt fails. Stored
// for audit; none of these values are user-facing.
type PaymentFailure struct {
	Code        string `bson:"code"`
	Source      string `bson:"source"`
	Step        string `bson:"step"`
	Reason      string `bson:"reason"`
	Description string `bson:"description"`
}

// Payment is the Razorpay sub-document on an order (schema.md §orders).
//
// Method and the detail fields (Last4, Network, VPA, Bank, Wallet) are empty
// until Razorpay reports them after the shopper pays. A pending_payment order
// has no method yet — that is expected, not an error.
//
// There is deliberately NO field for a card number. Card details are entered
// inside Razorpay's modal and never reach this server (schema.md §Razorpay).
// Only the non-sensitive metadata Razorpay reports back is stored: Last4 and
// Network for a card, VPA for UPI. These are method-scoped: a detail field for
// the wrong method is always rejected by Validate.
//
// CapturedAt uses *time.Time because nil is semantically distinct from a zero
// timestamp: a zero time.Time would marshal to a year-1 date, which queries
// would treat as a real capture time.
type Payment struct {
	Provider string        `bson:"provider"` // always "razorpay"
	Status   PaymentStatus `bson:"status"`
	Amount   int64         `bson:"amount"`   // paise — what Razorpay was asked to collect
	Currency string        `bson:"currency"` // always "INR"

	RazorpayOrderID   string `bson:"razorpayOrderId,omitempty"`   // absent before Razorpay confirms
	RazorpayPaymentID string `bson:"razorpayPaymentId,omitempty"` // set when a payment attaches
	RazorpaySignature string `bson:"razorpaySignature,omitempty"` // the callback HMAC, kept for audit

	// Method and Label are empty until Razorpay reports them.
	Method PaymentMethod `bson:"method,omitempty"`
	Label  string        `bson:"label,omitempty"`

	// Detail fields — each is set only for the relevant method.
	Last4   string `bson:"last4,omitempty"`   // card only — never the full number
	Network string `bson:"network,omitempty"` // card only
	Bank    string `bson:"bank,omitempty"`    // netbanking only
	Wallet  string `bson:"wallet,omitempty"`  // wallet only
	VPA     string `bson:"vpa,omitempty"`     // upi only

	Attempts   int             `bson:"attempts"`
	CapturedAt *time.Time      `bson:"capturedAt"` // nil until captured; *time.Time avoids year-1 dates
	Failure    *PaymentFailure `bson:"failure"`    // nil until a failure is reported
}

// NewPendingPayment builds the Payment sub-document at order creation time.
// Method and detail fields are left empty — Razorpay will report them later.
func NewPendingPayment(razorpayOrderID string, amount int64) Payment {
	return Payment{
		Provider:        providerRazorpay,
		Status:          PaymentStatusCreated,
		Amount:          amount,
		Currency:        currencyINR,
		RazorpayOrderID: razorpayOrderID,
	}
}

// Validate checks the payment sub-document is internally consistent.
//
// When Method is empty (pending state), only the gateway fields are checked and
// no method-detail fields (Last4, Network, VPA, Bank, Wallet) are permitted —
// they imply a method choice that has not yet been made.
//
// When Method is set, the label must match, and detail fields must be scoped to
// the right method: Last4/Network are card-only, VPA is upi-only, Bank is
// netbanking-only, Wallet is wallet-only. EMI and PayLater carry no detail.
func (p Payment) Validate() error {
	if p.Provider != providerRazorpay {
		return fmt.Errorf("order: unknown payment provider %q", p.Provider)
	}
	if !p.Status.Valid() {
		return fmt.Errorf("order: unknown payment status %q", p.Status)
	}
	if p.Amount <= 0 {
		return fmt.Errorf("order: payment amount must be positive, got %d", p.Amount)
	}
	if p.Currency != currencyINR {
		return fmt.Errorf("order: unknown payment currency %q", p.Currency)
	}
	// A freshly-built order is inserted before the Razorpay call links them
	// (the insert-before-Razorpay pattern); its RazorpayOrderID is intentionally
	// empty until SetRazorpayOrderID is called.
	if p.RazorpayOrderID == "" && p.Status != PaymentStatusCreated {
		return errors.New("order: payment razorpay order id is required")
	}

	if p.Method == "" {
		// Pending state: no method yet, so no detail fields are allowed.
		if p.Label != "" || p.Last4 != "" || p.Network != "" || p.Bank != "" || p.Wallet != "" || p.VPA != "" {
			return errors.New("order: pending payment must not carry method-detail fields")
		}
		return nil
	}

	if !p.Method.Valid() {
		return fmt.Errorf("order: unknown payment method %q", p.Method)
	}
	if p.Label != p.Method.Label() {
		return fmt.Errorf("order: payment label %q does not match method %q", p.Label, p.Method)
	}

	// Enforce method-scoped detail fields.
	switch p.Method {
	case PaymentCard:
		if p.VPA != "" || p.Bank != "" || p.Wallet != "" {
			return errors.New("order: card payment must not carry vpa, bank or wallet fields")
		}
		if p.Last4 != "" && !isFourDigits(p.Last4) {
			return fmt.Errorf("order: card last4 must be four digits, got %q", p.Last4)
		}
	case PaymentUPI:
		if p.Last4 != "" || p.Network != "" || p.Bank != "" || p.Wallet != "" {
			return errors.New("order: upi payment must not carry card, bank or wallet fields")
		}
	case PaymentNetbanking:
		if p.Last4 != "" || p.Network != "" || p.VPA != "" || p.Wallet != "" {
			return errors.New("order: netbanking payment must not carry card, upi or wallet fields")
		}
	case PaymentWallet:
		if p.Last4 != "" || p.Network != "" || p.VPA != "" || p.Bank != "" {
			return errors.New("order: wallet payment must not carry card, upi or bank fields")
		}
	case PaymentEMI, PaymentPayLater:
		if p.Last4 != "" || p.Network != "" || p.VPA != "" || p.Bank != "" || p.Wallet != "" {
			return fmt.Errorf("order: %s payment must not carry method-detail fields", p.Method)
		}
	}

	return nil
}

// isFourDigits reports whether s is exactly four ASCII digits.
func isFourDigits(s string) bool {
	if len(s) != 4 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ─── Order line ──────────────────────────────────────────────────────────────

// Line is one bought item, frozen at creation time. Name, Form, Grad and the
// money fields are COPIES: they are never re-read from the catalogue, so a
// later catalogue edit leaves the order untouched (schema.md decision 4).
// ProductID is retained only so an order can be traced back to what was sold.
type Line struct {
	ProductID catalogue.ID   `bson:"productId"`
	Name      string         `bson:"name"`
	Form      catalogue.Form `bson:"form"`
	Grad      string         `bson:"grad"`
	// UnitPrice and UnitMrp are the per-unit charged price and struck-through
	// MRP, in paise, at the moment of creation.
	UnitPrice int64 `bson:"unitPrice"`
	UnitMrp   int64 `bson:"unitMrp"`
	Qty       int   `bson:"qty"`
	// LineTotal is UnitPrice × Qty, stored rather than recomputed so a receipt
	// is self-contained.
	LineTotal int64 `bson:"lineTotal"`
}

// Validate checks a frozen line is internally consistent: blank identity,
// non-positive money, or a LineTotal that disagrees with UnitPrice × Qty.
func (l Line) Validate() error {
	if l.ProductID == "" {
		return errors.New("order: line product id is required")
	}
	if l.Name == "" {
		return errors.New("order: line name is required")
	}
	if !l.Form.Valid() {
		return fmt.Errorf("order: line %q: unknown form %q", l.ProductID, l.Form)
	}
	if l.Qty <= 0 {
		return fmt.Errorf("order: line %q: qty must be positive, got %d", l.ProductID, l.Qty)
	}
	if l.UnitPrice <= 0 {
		return fmt.Errorf("order: line %q: unit price must be positive, got %d", l.ProductID, l.UnitPrice)
	}
	if l.UnitMrp <= 0 {
		return fmt.Errorf("order: line %q: unit mrp must be positive, got %d", l.ProductID, l.UnitMrp)
	}
	if l.UnitPrice > l.UnitMrp {
		return fmt.Errorf("order: line %q: unit price %d exceeds mrp %d", l.ProductID, l.UnitPrice, l.UnitMrp)
	}
	if want := l.UnitPrice * int64(l.Qty); l.LineTotal != want {
		return fmt.Errorf("order: line %q: line total %d != unit price × qty (%d)", l.ProductID, l.LineTotal, want)
	}
	return nil
}

// ─── Totals ──────────────────────────────────────────────────────────────────

// Totals is the money summary of an order, in paise. It has the same five
// fields as cart.Totals but is a distinct type: an order is a snapshot and must
// not be coupled to the live cart's type. The cart→order conversion is 5.3's
// job.
type Totals struct {
	MRPTotal int64 `bson:"mrpTotal"`
	Subtotal int64 `bson:"subtotal"`
	Savings  int64 `bson:"savings"`
	Shipping int64 `bson:"shipping"`
	Total    int64 `bson:"total"`
}

// Validate checks the totals are non-negative and internally consistent:
// savings is exactly the MRP-to-subtotal gap and total is subtotal plus
// shipping. A zero subtotal is rejected — an order with no value would mean an
// empty cart was checked out.
func (t Totals) Validate() error {
	switch {
	case t.Subtotal <= 0:
		return fmt.Errorf("order: subtotal must be positive, got %d", t.Subtotal)
	case t.MRPTotal < t.Subtotal:
		return fmt.Errorf("order: mrp total %d is below subtotal %d", t.MRPTotal, t.Subtotal)
	case t.Shipping < 0:
		return fmt.Errorf("order: shipping is negative (%d)", t.Shipping)
	case t.Savings != t.MRPTotal-t.Subtotal:
		return fmt.Errorf("order: savings %d != mrp total − subtotal (%d)", t.Savings, t.MRPTotal-t.Subtotal)
	case t.Total != t.Subtotal+t.Shipping:
		return fmt.Errorf("order: total %d != subtotal + shipping (%d)", t.Total, t.Subtotal+t.Shipping)
	}
	return nil
}

// ─── Order ───────────────────────────────────────────────────────────────────

// Order is an order document as it sits in MongoDB (schema.md §orders). It is
// created in pending_payment and moves to placed when Razorpay confirms
// capture; payment_failed and expired release the stock reservation.
//
// Lines, Totals and ShippingAddress are snapshots frozen at creation time.
// PlacedAt and Payment.CapturedAt use *time.Time because nil is semantically
// distinct from a zero timestamp — a zero time.Time marshals to a year-1 date
// that a query on the field would misread as a real event.
//
// ShippingAddress reuses auth.Address. It is the same shape the shopper saved,
// and a copied value gives exactly the snapshot semantics an order needs. Its
// IsDefault flag is meaningless on a frozen order and simply ignored.
//
// There is no etaText field. The ETA is presentational and computed at response
// time (roadmap.md §Orders), never stored.
type Order struct {
	ID      bson.ObjectID `bson:"_id,omitempty"`
	OrderID string        `bson:"orderId"`
	UserID  bson.ObjectID `bson:"userId"`
	Status  Status        `bson:"status"`
	Lines   []Line        `bson:"lines"`
	Totals  Totals        `bson:"totals"`
	Payment Payment       `bson:"payment"`
	// ShippingAddress is a copy, not a reference.
	ShippingAddress auth.Address `bson:"shippingAddress"`

	// CustomerPhone is the delivery contact frozen at creation: the address's
	// own phone, or the account's when the address predates that field. It is
	// what the shipping label prints and what an operator calls when a delivery
	// goes wrong.
	//
	// omitempty because orders placed before this field existed have none, and
	// Validate tolerates that — they are real orders and must stay readable.
	CustomerPhone string `bson:"customerPhone,omitempty"`

	// Fulfilment is what a person did with the parcel, written only by the
	// admin order book and read by no shopper-facing endpoint. omitempty is
	// load-bearing: FulfilmentNone is the absence of the key, which is what
	// makes every pre-existing order correct without a migration.
	Fulfilment Fulfilment `bson:"fulfilment,omitempty"`

	CreatedAt time.Time  `bson:"createdAt"`
	ExpiresAt time.Time  `bson:"expiresAt"` // createdAt + 15 min; NOT a TTL index
	PlacedAt  *time.Time `bson:"placedAt"`  // nil until payment is captured
	UpdatedAt time.Time  `bson:"updatedAt"`
}

// ItemCount is the total number of units across all lines.
func (o Order) ItemCount() int {
	n := 0
	for _, l := range o.Lines {
		n += l.Qty
	}
	return n
}

// Validate checks an order is well-formed. The rules differ by status:
//
//   - pending_payment, payment_failed, expired: PlacedAt may be nil and the
//     payment method may be empty — money has not moved yet.
//   - placed and all subsequent states (packed, shipped, delivered, cancelled):
//     PlacedAt, payment method and payment capturedAt must all be present —
//     the order represents a completed purchase.
//
// This split is the point of the model rework: a single Validate that only
// understood a finished order would either reject every reservation or wave
// through a half-finished receipt.
func (o Order) Validate() error {
	if !ValidOrderID(o.OrderID) {
		return fmt.Errorf("order: order id %q does not match EFF-######", o.OrderID)
	}
	if o.UserID.IsZero() {
		return errors.New("order: user id is required")
	}
	if !o.Status.Valid() {
		return fmt.Errorf("order: unknown status %q", o.Status)
	}
	if !o.Fulfilment.Valid() {
		return fmt.Errorf("order: unknown fulfilment %q", o.Fulfilment)
	}
	// Checked only when set. Every order this system creates has one, but
	// orders written before the field existed do not, and rejecting those would
	// make a real past purchase unreadable. A malformed value is still caught —
	// a label printed with a mangled number is worse than one with none.
	if o.CustomerPhone != "" && !auth.ValidPhone(o.CustomerPhone) {
		return fmt.Errorf("order: customer phone %q is not a storable phone number", o.CustomerPhone)
	}
	// Fulfilment progress on an order nobody paid for would mean a parcel went
	// out for free. Only a placed order can carry one.
	if o.Fulfilment != FulfilmentNone && o.Status != StatusPlaced {
		return fmt.Errorf("order: %s order cannot carry fulfilment %q", o.Status, o.Fulfilment)
	}
	if len(o.Lines) == 0 {
		return errors.New("order: an order needs at least one line")
	}
	for _, l := range o.Lines {
		if err := l.Validate(); err != nil {
			return err
		}
	}
	if err := o.Totals.Validate(); err != nil {
		return err
	}
	if err := o.Payment.Validate(); err != nil {
		return err
	}

	// Payment.Amount is what we asked Razorpay to collect; Totals.Total is what
	// the cart came to. They must be equal — a mismatch is the difference
	// between charging ₹1,140 and charging ₹1, and it is the one inconsistency
	// no other check would catch (schema.md §Razorpay).
	if o.Payment.Amount != o.Totals.Total {
		return fmt.Errorf("order: payment amount %d != totals total %d", o.Payment.Amount, o.Totals.Total)
	}

	// Placed orders and later represent completed purchases and must carry
	// evidence that the money moved.
	switch o.Status {
	case StatusPlaced, StatusPacked, StatusShipped, StatusDelivered, StatusCancelled:
		if o.PlacedAt == nil {
			return errors.New("order: placed order is missing placedAt")
		}
		if o.Payment.Method == "" {
			return errors.New("order: placed order is missing payment method")
		}
		if o.Payment.CapturedAt == nil {
			return errors.New("order: placed order is missing payment capturedAt")
		}
		// A placed order requires a payment status consistent with money having
		// moved. captured is the normal case; refunded is allowed because a
		// captured payment can be refunded without un-happening the order.
		if o.Payment.Status != PaymentStatusCaptured && o.Payment.Status != PaymentStatusRefunded {
			return fmt.Errorf("order: placed order has non-terminal payment status %q", o.Payment.Status)
		}
	}

	return nil
}
