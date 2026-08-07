package order

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const paymentEventsCollection = "payment_events"

// PaymentEventsRepository reads and writes the payment_events collection.
// The only index is the implicit _id_ index — it is the whole dedupe mechanism
// (schema.md §payment_events).
type PaymentEventsRepository struct {
	col *mongo.Collection
}

// NewPaymentEventsRepository builds a repository over the given database.
func NewPaymentEventsRepository(db *mongo.Database) *PaymentEventsRepository {
	return &PaymentEventsRepository{col: db.Collection(paymentEventsCollection)}
}

// InsertEvent inserts e into payment_events. The _id is Razorpay's event id,
// so a duplicate-key error means this delivery has already been seen.
// Returns ErrDuplicateEvent on a collision; wraps other errors with context.
func (r *PaymentEventsRepository) InsertEvent(ctx context.Context, e PaymentEvent) error {
	_, err := r.col.InsertOne(ctx, e)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ErrDuplicateEvent
		}
		return fmt.Errorf("order: insert payment event: %w", err)
	}
	return nil
}

// GetEvent retrieves a payment event by its Razorpay event id. Called after a
// duplicate-key insert to inspect whether a prior delivery completed dispatch.
func (r *PaymentEventsRepository) GetEvent(ctx context.Context, eventID string) (PaymentEvent, error) {
	var e PaymentEvent
	err := r.col.FindOne(ctx, bson.D{{Key: "_id", Value: eventID}}).Decode(&e)
	if err != nil {
		return PaymentEvent{}, fmt.Errorf("order: get payment event: %w", err)
	}
	return e, nil
}

// MarkEventProcessed sets processed: true on a payment_events document.
// Called only after dispatch returns success so the flag reliably indicates
// that the business transition (MarkPlaced / MarkPaymentFailed) has been applied.
func (r *PaymentEventsRepository) MarkEventProcessed(ctx context.Context, eventID string) error {
	_, err := r.col.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: eventID}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "processed", Value: true}}}},
	)
	if err != nil {
		return fmt.Errorf("order: mark payment event processed: %w", err)
	}
	return nil
}
