package order

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Collection and field names, per schema.md §orders.
const (
	ordersCollection = "orders"

	fieldOrderID           = "orderId"
	fieldUserID            = "userId"
	fieldStatus            = "status"
	fieldCreatedAt         = "createdAt"
	fieldUpdatedAt         = "updatedAt"
	fieldPlacedAt          = "placedAt"
	fieldPayment           = "payment"
	fieldExpiresAt         = "expiresAt"
	fieldRazorpayOrderID   = "payment.razorpayOrderId"
	fieldRazorpayPaymentID = "payment.razorpayPaymentId"

	// Payment detail sub-fields, written on their own by FillPaymentDetail.
	fieldPaymentMethod  = "payment.method"
	fieldPaymentLabel   = "payment.label"
	fieldPaymentLast4   = "payment.last4"
	fieldPaymentNetwork = "payment.network"
	fieldPaymentBank    = "payment.bank"
	fieldPaymentWallet  = "payment.wallet"
	fieldPaymentVPA     = "payment.vpa"

	bsonOpSet = "$set" // used in every update to avoid 4× goconst lint hits
)

// ErrOrderNotFound is returned when no order matches the caller's query —
// including when the order exists but belongs to a different user. Callers
// must never distinguish the two cases (roadmap.md §GET /orders/{orderId}).
var ErrOrderNotFound = errors.New("order: not found")

// ErrDuplicateOrderID is returned by Create when the generated orderId
// collides with an existing one on the orderId_1 unique index. The service
// retries with a freshly generated id — that retry is the whole reason the
// index exists (schema.md §Index creation).
var ErrDuplicateOrderID = errors.New("order: orderId already exists")

// ErrPendingOrderExists is returned by Create when the userId_1 partial
// unique index fires — the shopper already holds a pending_payment reservation.
// The service must expire that order before inserting a new one. This must NOT
// be mapped to ErrDuplicateOrderID: they require completely different responses
// and a retry on ErrPendingOrderExists would loop forever.
var ErrPendingOrderExists = errors.New("order: user already has a pending order")

// Repository reads and writes the orders collection.
type Repository struct {
	col *mongo.Collection
}

// NewRepository builds a repository over the given database.
func NewRepository(db *mongo.Database) *Repository {
	return &Repository{col: db.Collection(ordersCollection)}
}

// Create inserts a new order document. Four unique indexes on orders can
// produce a duplicate-key error; the two that can fire during Create are
// mapped to distinct sentinels:
//
//   - orderId_1 → ErrDuplicateOrderID (service retries with a new id)
//   - userId_1  → ErrPendingOrderExists (user already has a reservation)
//
// Classification reads keyPattern from WriteException.WriteErrors[0].Raw —
// the field involved in the collision — not the index name. This is structural:
// the orderId_1 index has keyPattern {orderId:1}, the userId_1 partial index
// has keyPattern {userId:1}, and payment.razorpayOrderId_1 has keyPattern
// {"payment.razorpayOrderId":1}. An exact-key lookup on "orderId" (not a
// substring check) cannot match "payment.razorpayOrderId" regardless of casing.
func (r *Repository) Create(ctx context.Context, o Order) error {
	if _, err := r.col.InsertOne(ctx, o); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return classifyDupKeyError(err)
		}
		return fmt.Errorf("order: create: %w", err)
	}
	return nil
}

// classifyDupKeyError maps a duplicate-key WriteException to the sentinel for
// the index that fired. It reads keyPattern from WriteErrors[0].Raw, which the
// driver populates from the server's write error document. keyPattern contains
// the actual field names in the colliding index, so the check is immune to
// index naming conventions and to coincidental substring matches in the message.
func classifyDupKeyError(err error) error {
	var we mongo.WriteException
	if errors.As(err, &we) && len(we.WriteErrors) > 0 {
		kpVal, kpErr := we.WriteErrors[0].Raw.LookupErr("keyPattern")
		if kpErr == nil {
			if kp, ok := kpVal.DocumentOK(); ok {
				if _, e := kp.LookupErr(fieldOrderID); e == nil {
					return ErrDuplicateOrderID
				}
				if _, e := kp.LookupErr(fieldUserID); e == nil {
					return ErrPendingOrderExists
				}
			}
		}
	}
	// razorpayOrderId/razorpayPaymentId collision, or keyPattern unavailable.
	return fmt.Errorf("order: create: unexpected duplicate key: %w", err)
}

// ByOrderID returns the order identified by both orderID and userID in a
// single query. Another user's order is indistinguishable from one that does
// not exist: both return ErrOrderNotFound. Fetching by orderId alone and then
// comparing userId in Go is deliberately avoided — it leaks the difference
// through timing and through whatever ends up in logs.
func (r *Repository) ByOrderID(ctx context.Context, userID bson.ObjectID, orderID string) (Order, error) {
	var o Order
	err := r.col.FindOne(ctx, bson.D{
		{Key: fieldOrderID, Value: orderID},
		{Key: fieldUserID, Value: userID},
	}).Decode(&o)
	switch {
	case errors.Is(err, mongo.ErrNoDocuments):
		return Order{}, ErrOrderNotFound
	case err != nil:
		return Order{}, fmt.Errorf("order: by order id: %w", err)
	}
	return o, nil
}

// ListForUser returns only paid orders for a user (placed, packed, shipped,
// delivered, cancelled), sorted by createdAt descending. pending_payment,
// payment_failed, and expired orders are excluded. The list is an explicit $in
// rather than a $ne so a status added later is excluded until someone adds it
// here on purpose. Fulfilment is deliberately not part of this query: it is a
// separate field that no shopper-facing endpoint reads. Sorted on createdAt, not
// placedAt: consistent ordering across all status values without null scatter.
// No orders is []Order{}, nil — never nil and never an error.
func (r *Repository) ListForUser(ctx context.Context, userID bson.ObjectID) ([]Order, error) {
	cursor, err := r.col.Find(ctx,
		bson.D{
			{Key: fieldUserID, Value: userID},
			{Key: fieldStatus, Value: bson.D{{Key: opIn, Value: paidStatusValues()}}},
		},
		options.Find().SetSort(bson.D{{Key: fieldCreatedAt, Value: -1}}),
	)
	if err != nil {
		return nil, fmt.Errorf("order: list for user: %w", err)
	}

	var orders []Order
	if err := cursor.All(ctx, &orders); err != nil {
		return nil, fmt.Errorf("order: decode orders: %w", err)
	}
	if orders == nil {
		return []Order{}, nil
	}
	return orders, nil
}

// ByRazorpayOrderID returns the order whose payment.razorpayOrderId matches.
// Returns ErrOrderNotFound when no document matches — the same sentinel as
// ByOrderID so callers handle both the same way.
func (r *Repository) ByRazorpayOrderID(ctx context.Context, razorpayOrderID string) (Order, error) {
	var o Order
	err := r.col.FindOne(ctx, bson.D{
		{Key: fieldRazorpayOrderID, Value: razorpayOrderID},
	}).Decode(&o)
	switch {
	case errors.Is(err, mongo.ErrNoDocuments):
		return Order{}, ErrOrderNotFound
	case err != nil:
		return Order{}, fmt.Errorf("order: by razorpay order id: %w", err)
	}
	return o, nil
}

// MarkPaymentFailed performs the guarded transition from pending_payment to
// payment_failed. The filter carries status: "pending_payment" so a concurrent
// expiry or a duplicate webhook delivery cannot win twice. Returns (true, nil)
// when a document was modified; (false, nil) when the guard rejected the write.
// Stock must be released by the caller only when modified == true.
func (r *Repository) MarkPaymentFailed(ctx context.Context, orderID string, payment Payment, now time.Time) (bool, error) {
	res, err := r.col.UpdateOne(ctx,
		bson.D{
			{Key: fieldOrderID, Value: orderID},
			{Key: fieldStatus, Value: string(StatusPendingPayment)},
		},
		bson.D{{Key: bsonOpSet, Value: bson.D{
			{Key: fieldStatus, Value: string(StatusPaymentFailed)},
			{Key: fieldPayment, Value: payment},
			{Key: fieldUpdatedAt, Value: now},
		}}},
	)
	if err != nil {
		return false, fmt.Errorf("order: mark payment failed: %w", err)
	}
	return res.ModifiedCount == 1, nil
}

// MarkPlaced performs the guarded transition from pending_payment to placed.
// The filter carries status: "pending_payment" so whichever of the callback
// and the webhook arrives first wins; the second is a no-op that still returns
// success to its caller.
//
// now is the caller-supplied timestamp for updatedAt. Callers pass time.Now()
// at the call site so tests can supply a deterministic value and assert the
// exact timestamp written to the document.
//
// Returns (true, nil) when a document was modified and (false, nil) when the
// guard rejected the write. A zero modified count is NOT an error — it means
// the order was already placed by the other path (schema.md §Razorpay).
func (r *Repository) MarkPlaced(ctx context.Context, orderID string, payment Payment, placedAt, now time.Time) (bool, error) {
	res, err := r.col.UpdateOne(ctx,
		bson.D{
			{Key: fieldOrderID, Value: orderID},
			{Key: fieldStatus, Value: string(StatusPendingPayment)},
		},
		bson.D{{Key: bsonOpSet, Value: bson.D{
			{Key: fieldStatus, Value: string(StatusPlaced)},
			{Key: fieldPayment, Value: payment},
			{Key: fieldPlacedAt, Value: placedAt},
			{Key: fieldUpdatedAt, Value: now},
		}}},
	)
	if err != nil {
		return false, fmt.Errorf("order: mark placed: %w", err)
	}
	return res.ModifiedCount == 1, nil
}

// FillPaymentDetail stamps how an order was paid for onto an order that is
// ALREADY placed.
//
// This exists because of an asymmetry between the two capture paths. Razorpay's
// browser callback returns only an order id, a payment id and a signature — it
// genuinely does not know whether the shopper used netbanking or UPI. Only the
// webhook carries that. But the callback is synchronous with the browser and
// almost always wins the race to MarkPlaced, whose filter requires
// status: "pending_payment"; the webhook's richer update then matches nothing
// and its method is discarded. The result was every order placed with no
// payment method at all, and a receipt that cannot say how it was paid.
//
// The filter requires the method to still be empty, so this is idempotent and
// can never overwrite a method that was recorded correctly. Only the detail
// sub-fields and updatedAt are written — never status, never placedAt, never
// the amount, and never the callback's stored signature.
//
// Returns (false, nil) when there was nothing to fill, which is the normal case
// once an order already has its method.
func (r *Repository) FillPaymentDetail(ctx context.Context, orderID string, p Payment, now time.Time) (bool, error) {
	res, err := r.col.UpdateOne(ctx,
		bson.D{
			{Key: fieldOrderID, Value: orderID},
			{Key: fieldStatus, Value: string(StatusPlaced)},
			// Matches a missing key, an explicit null, and an empty string.
			{Key: fieldPaymentMethod, Value: bson.D{{Key: opIn, Value: bson.A{nil, ""}}}},
		},
		bson.D{{Key: bsonOpSet, Value: bson.D{
			{Key: fieldPaymentMethod, Value: string(p.Method)},
			{Key: fieldPaymentLabel, Value: p.Label},
			{Key: fieldPaymentLast4, Value: p.Last4},
			{Key: fieldPaymentNetwork, Value: p.Network},
			{Key: fieldPaymentBank, Value: p.Bank},
			{Key: fieldPaymentWallet, Value: p.Wallet},
			{Key: fieldPaymentVPA, Value: p.VPA},
			{Key: fieldUpdatedAt, Value: now},
		}}},
	)
	if err != nil {
		return false, fmt.Errorf("order: fill payment detail: %w", err)
	}
	return res.ModifiedCount == 1, nil
}

// ExpirePendingForUser atomically transitions the caller's pending_payment
// order (if any) to expired and returns it so the service can release its
// stock. The guard in the filter prevents a concurrent expiry from winning
// twice: only the goroutine whose update reports ModifiedCount == 1 owns the
// return value.
//
// Returns (Order{}, false, nil) when no pending order exists — that is not an
// error; it means either there was never one, or the sweeper beat us to it.
func (r *Repository) ExpirePendingForUser(ctx context.Context, userID bson.ObjectID, now time.Time) (Order, bool, error) {
	var o Order
	err := r.col.FindOneAndUpdate(ctx,
		bson.D{
			{Key: fieldUserID, Value: userID},
			{Key: fieldStatus, Value: string(StatusPendingPayment)},
		},
		bson.D{{Key: bsonOpSet, Value: bson.D{
			{Key: fieldStatus, Value: string(StatusExpired)},
			{Key: fieldUpdatedAt, Value: now},
		}}},
		options.FindOneAndUpdate().SetReturnDocument(options.Before),
	).Decode(&o)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return Order{}, false, nil
	}
	if err != nil {
		return Order{}, false, fmt.Errorf("order: expire pending for user: %w", err)
	}
	return o, true, nil
}

// SetRazorpayOrderID writes the razorpayOrderId returned by Razorpay's
// CreateOrder onto our pending_payment document. This is the second half of
// the insert-first pattern: we insert without the id (excluded from the
// partial unique index by $exists), call Razorpay, then commit the id.
func (r *Repository) SetRazorpayOrderID(ctx context.Context, orderID string, razorpayOrderID string) error {
	_, err := r.col.UpdateOne(ctx,
		bson.D{{Key: fieldOrderID, Value: orderID}},
		bson.D{{Key: bsonOpSet, Value: bson.D{
			{Key: fieldRazorpayOrderID, Value: razorpayOrderID},
		}}},
	)
	if err != nil {
		return fmt.Errorf("order: set razorpay order id: %w", err)
	}
	return nil
}

// FindExpiredPending returns at most limit orders that are still in
// pending_payment but whose expiresAt has passed. The sweeper uses this to
// discover which reservations to expire. Relies on the status_1_expiresAt_1
// partial index (schema.md §orders, Indexes) so the query never scans the
// full collection.
func (r *Repository) FindExpiredPending(ctx context.Context, now time.Time, limit int) ([]Order, error) {
	cursor, err := r.col.Find(ctx,
		bson.D{
			{Key: fieldStatus, Value: string(StatusPendingPayment)},
			{Key: fieldExpiresAt, Value: bson.D{{Key: "$lt", Value: now}}},
		},
		options.Find().SetLimit(int64(limit)),
	)
	if err != nil {
		return nil, fmt.Errorf("order: find expired pending: %w", err)
	}
	var orders []Order
	if err := cursor.All(ctx, &orders); err != nil {
		return nil, fmt.Errorf("order: decode expired pending: %w", err)
	}
	if orders == nil {
		return []Order{}, nil
	}
	return orders, nil
}

// MarkExpired performs the guarded transition from pending_payment to expired.
// The filter carries status: "pending_payment" so a concurrent callback,
// webhook, or another sweeper goroutine cannot win twice — only the first
// caller to report modifiedCount == 1 owns the expiry and must release stock.
//
// Returns (true, nil) when the document was modified; (false, nil) when the
// guard rejected the write (another process got there first). Stock must be
// released by the caller ONLY when modified == true.
func (r *Repository) MarkExpired(ctx context.Context, orderID string, now time.Time) (bool, error) {
	res, err := r.col.UpdateOne(ctx,
		bson.D{
			{Key: fieldOrderID, Value: orderID},
			{Key: fieldStatus, Value: string(StatusPendingPayment)},
		},
		bson.D{{Key: bsonOpSet, Value: bson.D{
			{Key: fieldStatus, Value: string(StatusExpired)},
			{Key: fieldUpdatedAt, Value: now},
		}}},
	)
	if err != nil {
		return false, fmt.Errorf("order: mark expired: %w", err)
	}
	return res.ModifiedCount == 1, nil
}

// EnsureIndexes creates the seven indexes schema.md §orders specifies. Four of
// them are correctness constraints, not optimisations — each one makes a
// specific race safe rather than merely fast. See schema.md §Index creation.
func (r *Repository) EnsureIndexes(ctx context.Context) error {
	_, err := r.col.Indexes().CreateMany(ctx, []mongo.IndexModel{
		// 1. Customer-facing id: unique so the generator's collision retry is
		//    correct rather than hopeful.
		{
			Keys:    bson.D{{Key: fieldOrderID, Value: 1}},
			Options: options.Index().SetName("orderId_1").SetUnique(true),
		},
		// 2. "My orders" list: createdAt desc so pending orders appear in the
		//    right place without an in-memory sort.
		{
			Keys:    bson.D{{Key: fieldUserID, Value: 1}, {Key: fieldCreatedAt, Value: -1}},
			Options: options.Index().SetName("userId_1_createdAt_-1"),
		},
		// 3. Callback/webhook resolution: unique so a replayed event cannot
		//    attach the same Razorpay order to a second document.
		{
			Keys: bson.D{{Key: fieldRazorpayOrderID, Value: 1}},
			Options: options.Index().
				SetName("payment.razorpayOrderId_1").
				SetUnique(true).
				SetPartialFilterExpression(bson.D{
					{Key: fieldRazorpayOrderID, Value: bson.D{{Key: "$exists", Value: true}}},
				}),
		},
		// 4. One payment per order: unique so a single Razorpay payment cannot
		//    be applied to two orders.
		{
			Keys: bson.D{{Key: fieldRazorpayPaymentID, Value: 1}},
			Options: options.Index().
				SetName("payment.razorpayPaymentId_1").
				SetUnique(true).
				SetPartialFilterExpression(bson.D{
					{Key: fieldRazorpayPaymentID, Value: bson.D{{Key: "$exists", Value: true}}},
				}),
		},
		// 5. At most one live reservation per shopper: unique on userId scoped
		//    to pending_payment. A double-click cannot reserve the same stock
		//    twice even if the service's expiry step races.
		{
			Keys: bson.D{{Key: fieldUserID, Value: 1}},
			Options: options.Index().
				SetName("userId_1").
				SetUnique(true).
				SetPartialFilterExpression(bson.D{
					{Key: fieldStatus, Value: string(StatusPendingPayment)},
				}),
		},
		// 6. Sweeper efficiency: lets the sweeper find abandoned reservations
		//    without scanning the whole collection.
		{
			Keys: bson.D{{Key: fieldStatus, Value: 1}, {Key: fieldExpiresAt, Value: 1}},
			Options: options.Index().
				SetName("status_1_expiresAt_1").
				SetPartialFilterExpression(bson.D{
					{Key: fieldStatus, Value: string(StatusPendingPayment)},
				}),
		},
		// 7. Admin order book: filters by status and pages backwards on
		//    createdAt. userId_1_createdAt_-1 cannot serve it — the admin query
		//    has no userId — so without this every page is a collection scan.
		{
			Keys:    bson.D{{Key: fieldStatus, Value: 1}, {Key: fieldCreatedAt, Value: -1}},
			Options: options.Index().SetName("status_1_createdAt_-1"),
		},
	})
	if err != nil {
		return fmt.Errorf("order: ensure indexes: %w", err)
	}
	return nil
}
