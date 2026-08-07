package order

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/enerzia/enerzia-be/internal/auth"
	"github.com/enerzia/enerzia-be/internal/cart"
	"github.com/enerzia/enerzia-be/internal/catalogue"
	"github.com/enerzia/enerzia-be/internal/razorpay"
)

// ReservationWindow is how long a pending_payment order holds its stock before
// the sweeper may expire it. 15 minutes matches the checkout UX.
const ReservationWindow = 15 * time.Minute

// maxIDRetries is how many times OpenCheckout will re-generate an EFF-######
// id before giving up. Five tries in a 1 M-id space means the collision
// probability only becomes non-negligible at ~50 M existing orders.
const maxIDRetries = 5

// Service-level errors. Each maps to exactly one API response; the handler
// never has to interpret a repository error directly.
var (
	// ErrCartEmpty means no lines were found for the shopper.
	ErrCartEmpty = errors.New("order: cart is empty")
	// ErrNoSavedAddress means the shopper has no addresses on file. Maps to
	// 422 so the frontend can distinguish "not found" from "none saved yet".
	ErrNoSavedAddress = errors.New("order: no saved address")
	// ErrAddressNotFound means a specific addressId was given but the shopper
	// does not own it (or it does not exist). Maps to 404.
	ErrAddressNotFound = errors.New("order: shipping address not found")
	// ErrGateway means Razorpay did not return a usable order id. The service
	// has already compensated (stock returned, our order expired).
	ErrGateway = errors.New("order: payment gateway unavailable")
	// ErrSignatureInvalid is returned when HMAC verification fails on a
	// payment callback, or when the razorpayOrderId in the callback does not
	// match the one stored on the order. One error for all three validation
	// failures (signature, id mismatch, amount mismatch) so the distinction
	// is not a security leak.
	ErrSignatureInvalid = errors.New("order: payment signature invalid")
	// ErrOrderNotPending is returned when ConfirmPayment is called on an order
	// that is neither pending_payment nor already placed.
	ErrOrderNotPending = errors.New("order: order is not awaiting payment")
)

// BlockingLineError is returned when a cart line prevents checkout.
type BlockingLineError struct{ Name string }

// Error implements error.
func (e *BlockingLineError) Error() string { return "order: blocking line: " + e.Name }

// StockUnavailableError is returned when a stock reservation fails mid-loop
// because another request took the last units between the availability check
// and the atomic decrement.
type StockUnavailableError struct{ Name string }

// Error implements error.
func (e *StockUnavailableError) Error() string { return "order: stock unavailable: " + e.Name }

// Store is the persistence surface the service needs. *Repository satisfies it;
// tests substitute a stub.
type Store interface {
	Create(ctx context.Context, o Order) error
	ByOrderID(ctx context.Context, userID bson.ObjectID, orderID string) (Order, error)
	ListForUser(ctx context.Context, userID bson.ObjectID) ([]Order, error)
	ByRazorpayOrderID(ctx context.Context, razorpayOrderID string) (Order, error)
	MarkPlaced(ctx context.Context, orderID string, payment Payment, placedAt, now time.Time) (bool, error)
	MarkPaymentFailed(ctx context.Context, orderID string, payment Payment, now time.Time) (bool, error)
	ExpirePendingForUser(ctx context.Context, userID bson.ObjectID, now time.Time) (Order, bool, error)
	SetRazorpayOrderID(ctx context.Context, orderID string, razorpayOrderID string) error
}

// CartLiner fetches resolved, live-priced cart lines for a user.
type CartLiner interface {
	Lines(ctx context.Context, userID bson.ObjectID) ([]cart.Line, error)
}

// CartClearer empties the shopper's cart after a successful payment. It is
// called as a best-effort step: a failure is logged but does not roll back
// the order transition. *cart.Service satisfies this interface.
type CartClearer interface {
	Clear(ctx context.Context, userID bson.ObjectID) (cart.View, error)
}

// AddressResolver picks the shipping address for an order.
type AddressResolver interface {
	AddressFor(ctx context.Context, userID bson.ObjectID, addressID *bson.ObjectID) (auth.Address, error)
}

// StockKeeper atomically increments and decrements product stock.
type StockKeeper interface {
	TakeStock(ctx context.Context, id catalogue.ID, qty int) error
	ReturnStock(ctx context.Context, id catalogue.ID, qty int) error
}

// ServiceConfig groups the dependencies injected into Service.
type ServiceConfig struct {
	Repo          Store
	Cart          CartLiner
	CartClearer   CartClearer
	Auth          AddressResolver
	Catalogue     StockKeeper
	Gateway       razorpay.Gateway
	Events        PaymentEventStore
	RazorpayKeyID string
	Logger        *slog.Logger
	// Now overrides the clock. Defaults to time.Now when nil.
	Now func() time.Time
}

// Service owns the rules for opening a checkout, confirming payment, and
// handling Razorpay webhooks.
type Service struct {
	repo          Store
	cart          CartLiner
	cartClearer   CartClearer
	auth          AddressResolver
	catalogue     StockKeeper
	gateway       razorpay.Gateway
	events        PaymentEventStore
	razorpayKeyID string
	logger        *slog.Logger
	now           func() time.Time
}

// NewService builds the order service from its dependencies.
func NewService(cfg ServiceConfig) *Service {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Service{
		repo:          cfg.Repo,
		cart:          cfg.Cart,
		cartClearer:   cfg.CartClearer,
		auth:          cfg.Auth,
		catalogue:     cfg.Catalogue,
		gateway:       cfg.Gateway,
		events:        cfg.Events,
		razorpayKeyID: cfg.RazorpayKeyID,
		logger:        cfg.Logger,
		now:           cfg.Now,
	}
}

// CheckoutResult is returned by OpenCheckout on success.
type CheckoutResult struct {
	Order           Order
	RazorpayKeyID   string
	RazorpayOrderID string
	Amount          int64
	Currency        string
}

// OpenCheckout creates a pending_payment reservation and a matching Razorpay
// order, following the six-step sequence in roadmap.md §POST /api/v1/orders.
//
// addressID is optional: when nil the shopper's default address is used.
func (s *Service) OpenCheckout(ctx context.Context, userID bson.ObjectID, addressID *bson.ObjectID) (CheckoutResult, error) {
	// 1. Fetch live cart lines (catalogue-authoritative prices).
	cartLines, err := s.cart.Lines(ctx, userID)
	if err != nil {
		return CheckoutResult{}, fmt.Errorf("order: open checkout: %w", err)
	}
	if len(cartLines) == 0 {
		return CheckoutResult{}, ErrCartEmpty
	}

	// 2. Every line must be purchasable right now.
	for _, l := range cartLines {
		if l.Blocking() {
			return CheckoutResult{}, &BlockingLineError{Name: l.Name}
		}
	}

	// 3. Resolve the shipping address.
	addr, err := s.auth.AddressFor(ctx, userID, addressID)
	if errors.Is(err, auth.ErrAddressNotFound) {
		if addressID == nil {
			return CheckoutResult{}, ErrNoSavedAddress
		}
		return CheckoutResult{}, ErrAddressNotFound
	}
	if err != nil {
		return CheckoutResult{}, fmt.Errorf("order: open checkout: %w", err)
	}

	now := s.now()

	// 4. Expire any live reservation the shopper already holds and return its
	// stock so it can be re-reserved under the new order.
	expired, found, err := s.repo.ExpirePendingForUser(ctx, userID, now)
	if err != nil {
		return CheckoutResult{}, fmt.Errorf("order: open checkout: %w", err)
	}
	if found {
		s.releaseLines(ctx, expired.Lines)
	}

	// Convert cart lines to order lines once so both reservation and
	// compensation use the same slice.
	orderLines := cartLinesToOrderLines(cartLines)

	// Compute totals from the live cart — catalogue-authoritative.
	ct := cart.ComputeTotals(cartLines)
	totals := Totals{
		MRPTotal: ct.MRPTotal,
		Subtotal: ct.Subtotal,
		Savings:  ct.Savings,
		Shipping: ct.Shipping,
		Total:    ct.Total,
	}

	// 5. Reserve stock one product at a time. The guard ($gte qty) in
	// TakeStock makes each decrement atomic; on any failure we restore whatever
	// was already taken.
	for i, ol := range orderLines {
		if takeErr := s.catalogue.TakeStock(ctx, ol.ProductID, ol.Qty); takeErr != nil {
			s.releaseLines(ctx, orderLines[:i])
			if errors.Is(takeErr, catalogue.ErrOutOfStock) {
				return CheckoutResult{}, &StockUnavailableError{Name: cartLines[i].Name}
			}
			return CheckoutResult{}, fmt.Errorf("order: open checkout: %w", takeErr)
		}
	}

	// 6. Insert the order. Payment is in its pending shape: RazorpayOrderID is
	// absent (omitempty in BSON), so the partial unique index on
	// payment.razorpayOrderId ($exists) does not fire.
	var (
		o       Order
		created bool
	)
	for range maxIDRetries {
		orderID, genErr := NewOrderID()
		if genErr != nil {
			s.releaseLines(ctx, orderLines)
			return CheckoutResult{}, fmt.Errorf("order: open checkout: %w", genErr)
		}
		o = Order{
			OrderID: orderID,
			UserID:  userID,
			Status:  StatusPendingPayment,
			Lines:   orderLines,
			Totals:  totals,
			Payment: Payment{
				Provider: providerRazorpay,
				Status:   PaymentStatusCreated,
				Amount:   totals.Total,
				Currency: currencyINR,
			},
			ShippingAddress: addr,
			CreatedAt:       now,
			ExpiresAt:       now.Add(ReservationWindow),
			UpdatedAt:       now,
		}
		if valErr := o.Validate(); valErr != nil {
			s.releaseLines(ctx, orderLines)
			return CheckoutResult{}, fmt.Errorf("order: open checkout: internal: %w", valErr)
		}
		if createErr := s.repo.Create(ctx, o); createErr != nil {
			if errors.Is(createErr, ErrDuplicateOrderID) {
				continue
			}
			s.releaseLines(ctx, orderLines)
			if errors.Is(createErr, ErrPendingOrderExists) {
				return CheckoutResult{}, ErrPendingOrderExists
			}
			return CheckoutResult{}, fmt.Errorf("order: open checkout: %w", createErr)
		}
		created = true
		break
	}
	if !created {
		s.releaseLines(ctx, orderLines)
		return CheckoutResult{}, fmt.Errorf("order: open checkout: id collision exhausted %d retries", maxIDRetries)
	}

	// 7. Create the Razorpay order. On failure, compensate: return all reserved
	// stock and expire the order we just inserted.
	rzpOrder, err := s.gateway.CreateOrder(ctx, razorpay.CreateOrderRequest{
		Amount:   totals.Total,
		Currency: currencyINR,
		Receipt:  o.OrderID,
	})
	if err != nil {
		s.compensateAfterInsert(ctx, userID, orderLines)
		return CheckoutResult{}, ErrGateway
	}

	// 8. Commit the Razorpay order id onto our document.
	if err := s.repo.SetRazorpayOrderID(ctx, o.OrderID, rzpOrder.ID); err != nil {
		s.compensateAfterInsert(ctx, userID, orderLines)
		return CheckoutResult{}, ErrGateway
	}

	return CheckoutResult{
		Order:           o,
		RazorpayKeyID:   s.razorpayKeyID,
		RazorpayOrderID: rzpOrder.ID,
		Amount:          totals.Total,
		Currency:        currencyINR,
	}, nil
}

// compensateAfterInsert releases reserved stock and expires the order we
// created, called when a step after insertion fails.
func (s *Service) compensateAfterInsert(ctx context.Context, userID bson.ObjectID, lines []Line) {
	s.releaseLines(ctx, lines)
	if _, _, err := s.repo.ExpirePendingForUser(ctx, userID, s.now()); err != nil {
		s.logger.ErrorContext(ctx, "order: compensate: expire pending", slog.Any("error", err))
	}
}

// releaseLines puts back each line's stock, logging but not surfacing errors
// so a compensation path never fails because a stock return failed.
func (s *Service) releaseLines(ctx context.Context, lines []Line) {
	for _, l := range lines {
		if err := s.catalogue.ReturnStock(ctx, l.ProductID, l.Qty); err != nil {
			s.logger.ErrorContext(ctx, "order: compensate: return stock", slog.Any("error", err))
		}
	}
}

// ConfirmPayment verifies the Razorpay callback and transitions the order from
// pending_payment to placed. It is idempotent: if the webhook already placed
// the order, it returns the current state with 200.
//
// The signature check, id-match check and amount-match check all return the
// same ErrSignatureInvalid so the distinction is not a security leak.
func (s *Service) ConfirmPayment(
	ctx context.Context,
	userID bson.ObjectID,
	orderID, razorpayOrderID, razorpayPaymentID, signature string,
) (Order, error) {
	// 1. Fetch order — ErrOrderNotFound if it does not exist or belongs to
	// another user.
	o, err := s.repo.ByOrderID(ctx, userID, orderID)
	if err != nil {
		return Order{}, err
	}

	// 2. Idempotent: already placed (webhook beat the callback, or a retry).
	if o.Status == StatusPlaced {
		return o, nil
	}

	// 3. Guard: only pending_payment can transition to placed.
	if o.Status != StatusPendingPayment {
		return Order{}, ErrOrderNotPending
	}

	// 4. Verify the HMAC. Log the failure category but surface only one message.
	if sigErr := s.gateway.VerifyCallbackSignature(razorpayOrderID, razorpayPaymentID, signature); sigErr != nil {
		s.logger.WarnContext(ctx, "order: callback: signature mismatch",
			slog.String("orderId", orderID),
		)
		return Order{}, ErrSignatureInvalid
	}

	// 5. Verify the razorpayOrderId in the callback matches our document.
	// This catches a replay of a valid (signature, paymentId) pair against a
	// different order (id mismatch and amount mismatch both caught here since
	// our Razorpay order was created with our total).
	if o.Payment.RazorpayOrderID != razorpayOrderID {
		s.logger.WarnContext(ctx, "order: callback: razorpay order id mismatch",
			slog.String("orderId", orderID),
		)
		return Order{}, ErrSignatureInvalid
	}

	now := s.now()
	payment := Payment{
		Provider:          providerRazorpay,
		Status:            PaymentStatusCaptured,
		Amount:            o.Payment.Amount,
		Currency:          o.Payment.Currency,
		RazorpayOrderID:   razorpayOrderID,
		RazorpayPaymentID: razorpayPaymentID,
		RazorpaySignature: signature,
		Attempts:          o.Payment.Attempts + 1,
		CapturedAt:        &now,
	}

	// 6. Guarded transition: only one of (callback, webhook) modifies the row.
	modified, err := s.repo.MarkPlaced(ctx, orderID, payment, now, now)
	if err != nil {
		return Order{}, fmt.Errorf("order: confirm payment: %w", err)
	}

	// 7. Webhook beat us to the transition — re-fetch to get its payment details.
	if !modified {
		return s.repo.ByOrderID(ctx, userID, orderID)
	}

	// 8. Best-effort cart clear: a failure is logged, not surfaced.
	if s.cartClearer != nil {
		if _, clearErr := s.cartClearer.Clear(ctx, userID); clearErr != nil {
			s.logger.ErrorContext(ctx, "order: confirm payment: clear cart",
				slog.String("orderId", orderID),
				slog.Any("error", clearErr),
			)
		}
	}

	// Return the placed order built from our in-memory snapshot.
	o.Status = StatusPlaced
	o.Payment = payment
	o.PlacedAt = &now
	o.UpdatedAt = now
	return o, nil
}

// ListOrders returns all non-expired orders for the user, newest first.
// Expired orders are excluded by the repository query (schema.md §orders
// lifecycle — expired is deliberately omitted from the shopper's history
// because nothing happened from their perspective).
func (s *Service) ListOrders(ctx context.Context, userID bson.ObjectID) ([]Order, error) {
	return s.repo.ListForUser(ctx, userID)
}

// GetOrder returns the order identified by orderID scoped to userID.
// Returns ErrOrderNotFound when the order does not exist or belongs to another
// user — the repository enforces both in a single query, so the service never
// compares userId in Go (roadmap.md §GET /api/v1/orders/{orderId}).
func (s *Service) GetOrder(ctx context.Context, userID bson.ObjectID, orderID string) (Order, error) {
	return s.repo.ByOrderID(ctx, userID, orderID)
}

// cartLinesToOrderLines converts live cart lines to the frozen order line
// shape. Name, prices and quantity are snapshots; stock and availability flags
// are dropped because they belong to the catalogue, not the order.
func cartLinesToOrderLines(cartLines []cart.Line) []Line {
	out := make([]Line, len(cartLines))
	for i, l := range cartLines {
		out[i] = Line{
			ProductID: l.ProductID,
			Name:      l.Name,
			Form:      l.Form,
			Grad:      l.Grad,
			UnitPrice: l.UnitPrice,
			UnitMrp:   l.UnitMRP,
			Qty:       l.Qty,
			LineTotal: l.LineTotal,
		}
	}
	return out
}
