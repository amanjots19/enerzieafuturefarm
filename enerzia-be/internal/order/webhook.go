package order

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
)

// ErrWebhookSignatureInvalid is returned by HandleWebhook when the
// X-Razorpay-Signature HMAC does not match RAZORPAY_WEBHOOK_SECRET.
// Maps to 400 BAD_REQUEST in the handler — the signature is the only thing
// between a public URL and a forged "payment captured" (schema.md §Razorpay).
var ErrWebhookSignatureInvalid = errors.New("order: webhook signature invalid")

// ─── Razorpay webhook body types ─────────────────────────────────────────────

// webhookPaymentEntity holds the fields this handler reads from
// payload.payment.entity. Extra fields sent by Razorpay are silently ignored.
type webhookPaymentEntity struct {
	ID               string `json:"id"`
	OrderID          string `json:"order_id"`
	Amount           int64  `json:"amount"`
	Method           string `json:"method"`
	VPA              string `json:"vpa"`
	Last4            string `json:"last4"`
	Network          string `json:"network"`
	Bank             string `json:"bank"`
	Wallet           string `json:"wallet"`
	ErrorCode        string `json:"error_code"`
	ErrorDescription string `json:"error_description"`
	ErrorSource      string `json:"error_source"`
	ErrorStep        string `json:"error_step"`
	ErrorReason      string `json:"error_reason"`
}

type webhookPaymentWrapper struct {
	Entity webhookPaymentEntity `json:"entity"`
}

type webhookPayloadFields struct {
	Payment *webhookPaymentWrapper `json:"payment"`
}

type webhookBody struct {
	Event   string               `json:"event"`
	Payload webhookPayloadFields `json:"payload"`
}

// ─── HandleWebhook ────────────────────────────────────────────────────────────

// HandleWebhook verifies, deduplicates, and applies a Razorpay webhook event.
//
// rawBody must be the exact bytes from io.ReadAll on the request body — the
// HMAC is computed over them before any JSON parsing, so they must not be
// decoded and re-encoded (schema.md §Razorpay, "Verifying the webhook").
//
// Returns ErrWebhookSignatureInvalid when the HMAC does not match.
// Returns nil for every other outcome including unknown events, already-deduped
// events, and orders that cannot be found — the caller must respond 200.
// Returns a non-nil, non-sentinel error only on genuine database failures where
// a Razorpay retry would help (roadmap.md §POST /webhooks/razorpay).
func (s *Service) HandleWebhook(ctx context.Context, rawBody []byte, eventID, sig string) error {
	// 1. Verify signature over raw bytes — nothing about the body is trusted
	// until this passes (schema.md §Razorpay, "Verifying the webhook").
	if err := s.gateway.VerifyWebhookSignature(rawBody, sig); err != nil {
		return ErrWebhookSignatureInvalid
	}

	// 2. Compute payload digest. The raw payload is never stored; only the
	// digest is kept so a disputed delivery can be verified (schema.md §payment_events).
	sum := sha256.Sum256(rawBody)
	digest := "sha256:" + hex.EncodeToString(sum[:])

	// 3. Parse the body. Safe now — the HMAC verified it came from Razorpay.
	var body webhookBody
	if err := json.Unmarshal(rawBody, &body); err != nil {
		s.logger.WarnContext(ctx, "order: webhook: malformed body after signature verification",
			slog.String("eventId", eventID))
		return nil //nolint:nilerr // a body that passes HMAC but fails JSON is Razorpay's problem; 200 so they don't retry
	}

	var razorpayOrderID, razorpayPaymentID string
	if body.Payload.Payment != nil {
		razorpayOrderID = body.Payload.Payment.Entity.OrderID
		razorpayPaymentID = body.Payload.Payment.Entity.ID
	}

	// 4. Insert-first dedupe with Processed: false.
	// The insert is still the dedupe mechanism — two concurrent retries cannot
	// both win. Processed is set true only AFTER dispatch succeeds so a delivery
	// that fails mid-dispatch leaves the flag false and allows the next retry to
	// reprocess (schema.md §payment_events).
	event := PaymentEvent{
		ID:                eventID,
		Event:             body.Event,
		RazorpayOrderID:   razorpayOrderID,
		RazorpayPaymentID: razorpayPaymentID,
		ReceivedAt:        s.now(),
		Processed:         false,
		PayloadDigest:     digest,
	}
	if err := s.events.InsertEvent(ctx, event); err != nil {
		if !errors.Is(err, ErrDuplicateEvent) {
			return fmt.Errorf("order: webhook: record event: %w", err)
		}
		// Duplicate delivery: a prior attempt reached the insert step. Load the
		// existing record to decide whether dispatch completed.
		existing, getErr := s.events.GetEvent(ctx, eventID)
		if getErr != nil {
			return fmt.Errorf("order: webhook: check existing event: %w", getErr)
		}
		if existing.Processed {
			return nil // dispatch already applied — safe to acknowledge
		}
		// Processed: false means the prior delivery failed after the insert but
		// before (or during) dispatch. Fall through and reprocess: every
		// transition is guarded on status: "pending_payment", so a second
		// application is a safe no-op that reports not-modified.
	}

	// 5. Dispatch on event type.
	var dispatchErr error
	switch body.Event {
	case "payment.captured", "order.paid":
		// order.paid is treated as a belt-and-braces duplicate of capture
		// (schema.md §Razorpay, "Events consumed").
		dispatchErr = s.handleWebhookCapture(ctx, razorpayOrderID, razorpayPaymentID, body.Payload.Payment)
	case "payment.failed":
		dispatchErr = s.handleWebhookFailed(ctx, razorpayOrderID, razorpayPaymentID, body.Payload.Payment)
	}
	if dispatchErr != nil {
		// Event stays Processed: false so the next Razorpay retry finds the
		// existing record, sees Processed: false, and reprocesses.
		return dispatchErr
	}

	// 6. Mark the event processed only after dispatch succeeds.
	// A failure here is logged but does not revert the transition: the order
	// is already placed or failed. A future retry would find Processed: false,
	// apply the guarded (no-op) transition again, and reattempt the mark.
	if err := s.events.MarkEventProcessed(ctx, eventID); err != nil {
		s.logger.ErrorContext(ctx, "order: webhook: mark event processed",
			slog.String("eventId", eventID),
			slog.Any("error", err),
		)
	}
	return nil
}

// handleWebhookCapture applies a payment.captured or order.paid event: moves
// the order from pending_payment to placed and clears the cart.
func (s *Service) handleWebhookCapture(
	ctx context.Context,
	razorpayOrderID, razorpayPaymentID string,
	pw *webhookPaymentWrapper,
) error {
	if razorpayOrderID == "" {
		s.logger.WarnContext(ctx, "order: webhook: capture missing razorpayOrderId; skipping")
		return nil
	}

	o, err := s.repo.ByRazorpayOrderID(ctx, razorpayOrderID)
	if err != nil {
		if errors.Is(err, ErrOrderNotFound) {
			s.logger.WarnContext(ctx, "order: webhook: capture: order not found",
				slog.String("razorpayOrderId", razorpayOrderID))
			return nil
		}
		return fmt.Errorf("order: webhook: resolve order: %w", err)
	}

	// Amount assertion — the only point where the charged amount can be verified
	// against an independent source. A mismatch means something is wrong that no
	// other check catches (schema.md §Razorpay, "The amount is ours").
	if pw != nil && pw.Entity.Amount != o.Totals.Total {
		s.logger.WarnContext(ctx, "order: webhook: SECURITY — capture amount mismatch",
			slog.String("orderId", o.OrderID),
			slog.Int64("webhookAmount", pw.Entity.Amount),
			slog.Int64("orderTotal", o.Totals.Total),
		)
		return nil // recorded; refuse the transition
	}

	now := s.now()

	var method PaymentMethod
	if pw != nil {
		method = PaymentMethod(pw.Entity.Method)
	}
	payment := Payment{
		Provider:          providerRazorpay,
		Status:            PaymentStatusCaptured,
		Amount:            o.Payment.Amount,
		Currency:          currencyINR,
		RazorpayOrderID:   razorpayOrderID,
		RazorpayPaymentID: razorpayPaymentID,
		Method:            method,
		Label:             method.Label(),
		Attempts:          o.Payment.Attempts + 1,
		CapturedAt:        &now,
	}
	if pw != nil {
		payment.Last4 = pw.Entity.Last4
		payment.Network = pw.Entity.Network
		payment.Bank = pw.Entity.Bank
		payment.Wallet = pw.Entity.Wallet
		payment.VPA = pw.Entity.VPA
	}

	// Guarded transition — whichever of the callback and the webhook arrives
	// first wins; the second is a no-op. Stock is NOT touched here: it was
	// already reserved at creation (schema.md §Razorpay, "The callback and the
	// webhook race").
	modified, err := s.repo.MarkPlaced(ctx, o.OrderID, payment, now, now)
	if err != nil {
		return fmt.Errorf("order: webhook: mark placed: %w", err)
	}

	// The callback almost always wins the race above, and it cannot know how the
	// shopper paid — Razorpay's browser response carries only ids and a
	// signature. Only this event does. Without this the method is discarded and
	// every receipt is left unable to say how it was paid.
	//
	// Guarded on the method still being empty, so a redelivery cannot overwrite
	// a value already recorded, and so this is a no-op once it has run.
	if !modified && method != "" {
		filled, fillErr := s.repo.FillPaymentDetail(ctx, o.OrderID, payment, now)
		switch {
		case fillErr != nil:
			// Worth an error line, but not worth a non-2xx: the order is placed
			// and the money is captured. A retry would only re-attempt this.
			s.logger.ErrorContext(ctx, "order: webhook: fill payment detail",
				slog.String("orderId", o.OrderID), slog.Any("error", fillErr))
		case filled:
			s.logger.InfoContext(ctx, "order: webhook: payment detail filled in after callback",
				slog.String("orderId", o.OrderID), slog.String("method", string(method)))
		}
	}

	// Side effects of a capture, both gated on modified so they happen exactly
	// once however many times Razorpay redelivers this event.
	if modified {
		// Built from the post-transition view: o was read BEFORE MarkPlaced, so
		// emailing it would tell the customer their order is awaiting payment.
		placed := o
		placed.Status = StatusPlaced
		placed.Payment = payment
		placed.PlacedAt = &now
		placed.UpdatedAt = now
		s.sendConfirmation(ctx, placed)
	}

	// Best-effort cart clear. A failure is logged but does not un-place the
	// order or cause a retry. The userID comes from the resolved order document
	// — the webhook carries no auth token (schema.md §Razorpay).
	if modified && s.cartClearer != nil {
		if _, clearErr := s.cartClearer.Clear(ctx, o.UserID); clearErr != nil {
			s.logger.ErrorContext(ctx, "order: webhook: clear cart",
				slog.String("orderId", o.OrderID),
				slog.Any("error", clearErr),
			)
		}
	}

	return nil
}

// handleWebhookFailed applies a payment.failed event: moves the order from
// pending_payment to payment_failed and releases its stock reservation.
func (s *Service) handleWebhookFailed(
	ctx context.Context,
	razorpayOrderID, razorpayPaymentID string,
	pw *webhookPaymentWrapper,
) error {
	if razorpayOrderID == "" {
		s.logger.WarnContext(ctx, "order: webhook: failed missing razorpayOrderId; skipping")
		return nil
	}

	o, err := s.repo.ByRazorpayOrderID(ctx, razorpayOrderID)
	if err != nil {
		if errors.Is(err, ErrOrderNotFound) {
			s.logger.WarnContext(ctx, "order: webhook: failed: order not found",
				slog.String("razorpayOrderId", razorpayOrderID))
			return nil
		}
		return fmt.Errorf("order: webhook: resolve order: %w", err)
	}

	now := s.now()
	payment := Payment{
		Provider:          providerRazorpay,
		Status:            PaymentStatusFailed,
		Amount:            o.Payment.Amount,
		Currency:          currencyINR,
		RazorpayOrderID:   razorpayOrderID,
		RazorpayPaymentID: razorpayPaymentID,
		Attempts:          o.Payment.Attempts + 1,
	}
	if pw != nil {
		payment.Failure = &PaymentFailure{
			Code:        pw.Entity.ErrorCode,
			Source:      pw.Entity.ErrorSource,
			Step:        pw.Entity.ErrorStep,
			Reason:      pw.Entity.ErrorReason,
			Description: pw.Entity.ErrorDescription,
		}
	}

	// Guarded transition — only modifies when the order is still
	// pending_payment. Stock is released only when we actually made the
	// transition; releasing on a no-op would return units a paid order now
	// owns (schema.md §orders, sweeper section).
	modified, err := s.repo.MarkPaymentFailed(ctx, o.OrderID, payment, now)
	if err != nil {
		return fmt.Errorf("order: webhook: mark payment failed: %w", err)
	}
	if modified {
		s.releaseLines(ctx, o.Lines)
	}

	return nil
}
