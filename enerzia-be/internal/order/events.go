package order

import (
	"context"
	"errors"
	"time"
)

// PaymentEvent is one Razorpay webhook delivery recorded in the payment_events
// collection (schema.md §payment_events). The _id is Razorpay's own event id
// so a duplicate-key error on insert is the dedupe signal — two concurrent
// retries of the same delivery cannot both succeed.
//
// The raw payload is never stored, only a SHA-256 digest of it, so a webhook
// body full of payment metadata is not kept at rest.
type PaymentEvent struct {
	ID                string    `bson:"_id"`
	Event             string    `bson:"event"`
	OrderID           string    `bson:"orderId"` // our EFF-###### id; empty if the order was not found
	RazorpayOrderID   string    `bson:"razorpayOrderId"`
	RazorpayPaymentID string    `bson:"razorpayPaymentId"`
	ReceivedAt        time.Time `bson:"receivedAt"`
	Processed         bool      `bson:"processed"`
	PayloadDigest     string    `bson:"payloadDigest"` // "sha256:<hex>", never the raw payload
}

// ErrDuplicateEvent is returned by PaymentEventStore.InsertEvent when a
// document with the same _id already exists. The handler must return 200
// without retrying the transition.
var ErrDuplicateEvent = errors.New("order: payment event already processed")

// PaymentEventStore is the persistence surface for webhook dedupe.
// *PaymentEventsRepository satisfies it; tests substitute a stub.
type PaymentEventStore interface {
	InsertEvent(ctx context.Context, e PaymentEvent) error
	// GetEvent loads the stored event by its Razorpay event id.
	// Called on a duplicate-key response to inspect whether a prior delivery
	// completed dispatch (Processed: true) or failed mid-way (Processed: false).
	GetEvent(ctx context.Context, eventID string) (PaymentEvent, error)
	// MarkEventProcessed sets Processed: true after dispatch succeeds.
	// A separate write rather than a field on the insert so the flag only
	// moves to true once the transition has actually been applied.
	MarkEventProcessed(ctx context.Context, eventID string) error
}
