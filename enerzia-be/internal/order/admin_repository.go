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

// fieldFulfilment is the fulfilment key on an order document.
const fieldFulfilment = "fulfilment"

// Query operators, named so a typo is a compile error rather than a filter that
// silently matches everything.
const (
	opIn  = "$in"
	opNin = "$nin"
	opLt  = "$lt"
	opOr  = "$or"
)

// AdminListDefaultLimit and AdminListMaxLimit bound a page of the order book
// (roadmap.md §GET /api/v1/admin/orders).
const (
	AdminListDefaultLimit = 50
	AdminListMaxLimit     = 200
)

// paidStatuses is what "an order you paid for" means, in one place.
//
// Both the shopper's list and the admin order book default to it, and they must
// not drift: a status in one and not the other would show an operator an order
// its buyer cannot see. It is an explicit list rather than "everything except
// the unpaid ones" so a status added later is excluded until someone adds it
// here deliberately (task 9.6).
var paidStatuses = []Status{
	StatusPlaced,
	StatusPacked,
	StatusShipped,
	StatusDelivered,
	StatusCancelled,
}

// PaidStatuses returns a copy of the statuses that count as a purchase.
// A copy, so a caller cannot reorder or truncate the query both lists depend on.
func PaidStatuses() []Status {
	out := make([]Status, len(paidStatuses))
	copy(out, paidStatuses)
	return out
}

// paidStatusValues is paidStatuses as BSON strings, for an $in.
func paidStatusValues() bson.A {
	out := make(bson.A, len(paidStatuses))
	for i, s := range paidStatuses {
		out[i] = string(s)
	}
	return out
}

// namedFulfilments is every fulfilment value that is actually stored. It
// excludes FulfilmentNone, which is the *absence* of the key — see the $nin in
// fulfilmentClause.
var namedFulfilments = []Fulfilment{FulfilmentPacked, FulfilmentInTransit, FulfilmentShipped}

// AdminFilter narrows a page of the admin order book.
//
// An empty Statuses or Fulfilments means "do not filter on this at all", which
// is what ?status=all asks for. It does NOT mean "match nothing" — a caller
// that wants the default paid set passes it explicitly.
type AdminFilter struct {
	Statuses    []Status
	Fulfilments []Fulfilment
	// Before is the cursor: return only orders created strictly before it.
	Before *time.Time
	// Limit caps the page. Zero or negative falls back to AdminListDefaultLimit.
	Limit int
}

// ListAll returns a page of every shopper's orders, newest first, for the admin
// order book (roadmap.md §GET /api/v1/admin/orders).
//
// Paging is a cursor on createdAt rather than a skip/offset. Orders arrive
// while the book is being read, and an offset would silently duplicate or skip
// a row when that happens — on this screen that is an order shipped twice, or
// never.
//
// No orders is []Order{}, nil — never nil and never an error.
func (r *Repository) ListAll(ctx context.Context, f AdminFilter) ([]Order, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = AdminListDefaultLimit
	}

	filter := bson.D{}
	if len(f.Statuses) > 0 {
		vals := make(bson.A, len(f.Statuses))
		for i, s := range f.Statuses {
			vals[i] = string(s)
		}
		filter = append(filter, bson.E{Key: fieldStatus, Value: bson.D{{Key: opIn, Value: vals}}})
	}
	if clause, ok := fulfilmentClause(f.Fulfilments); ok {
		filter = append(filter, clause)
	}
	if f.Before != nil {
		filter = append(filter, bson.E{
			Key:   fieldCreatedAt,
			Value: bson.D{{Key: opLt, Value: *f.Before}},
		})
	}

	cursor, err := r.col.Find(ctx, filter,
		options.Find().
			SetSort(bson.D{{Key: fieldCreatedAt, Value: -1}}).
			SetLimit(int64(limit)),
	)
	if err != nil {
		return nil, fmt.Errorf("order: list all: %w", err)
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

// ByOrderIDUnscoped returns an order by its customer-facing id, with **no user
// scoping** — the admin order book must reach any shopper's order.
//
// The name says "unscoped" deliberately. Repository.ByOrderID takes a userID
// and filters on it, and that scoping is the only thing stopping one shopper
// reading another's order. A call site that wants the scoped version and
// reaches for this one by mistake should read wrongly at a glance.
//
// Returns ErrOrderNotFound when nothing matches, the same sentinel as
// ByOrderID, so callers handle both the same way.
func (r *Repository) ByOrderIDUnscoped(ctx context.Context, orderID string) (Order, error) {
	var o Order
	err := r.col.FindOne(ctx, bson.D{{Key: fieldOrderID, Value: orderID}}).Decode(&o)
	switch {
	case errors.Is(err, mongo.ErrNoDocuments):
		return Order{}, ErrOrderNotFound
	case err != nil:
		return Order{}, fmt.Errorf("order: by order id unscoped: %w", err)
	}
	return o, nil
}

// SetFulfilment advances an order's fulfilment state, guarded.
//
// The filter carries the state the caller believes the order is in, so two
// operators clicking at once cannot both advance it: the second update matches
// nothing and reports false. Without that guard, a read-then-write would let
// both succeed and skip a step — the parcel would go from packed to shipped
// with nobody having marked it in transit.
//
// status: "placed" is in the filter for the same reason. An order that expired
// or failed between the read and the write must not acquire fulfilment
// progress.
//
// Only fulfilment and updatedAt are written. Never status, never stock, never
// money.
//
// Returns (false, nil) when the guard rejected the write — the caller reports a
// conflict rather than retrying.
func (r *Repository) SetFulfilment(
	ctx context.Context,
	orderID string,
	from, to Fulfilment,
	now time.Time,
) (bool, error) {
	filter := bson.D{
		{Key: fieldOrderID, Value: orderID},
		{Key: fieldStatus, Value: string(StatusPlaced)},
	}
	// FulfilmentNone is the absence of the key, so it cannot be matched by
	// equality — same reasoning as fulfilmentClause.
	if from == FulfilmentNone {
		stored := make(bson.A, len(namedFulfilments))
		for i, f := range namedFulfilments {
			stored[i] = string(f)
		}
		filter = append(filter, bson.E{
			Key:   fieldFulfilment,
			Value: bson.D{{Key: opNin, Value: stored}},
		})
	} else {
		filter = append(filter, bson.E{Key: fieldFulfilment, Value: string(from)})
	}

	res, err := r.col.UpdateOne(ctx, filter, bson.D{{Key: bsonOpSet, Value: bson.D{
		{Key: fieldFulfilment, Value: string(to)},
		{Key: fieldUpdatedAt, Value: now},
	}}})
	if err != nil {
		return false, fmt.Errorf("order: set fulfilment: %w", err)
	}
	return res.ModifiedCount == 1, nil
}

// fulfilmentClause builds the fulfilment predicate, or reports false when there
// is nothing to filter on.
//
// FulfilmentNone cannot be matched by equality: it is the absence of the key,
// because Order.Fulfilment is written with omitempty. Matching it as $nin over
// the stored values catches all three shapes an untouched order can have —
// missing, null, or an empty string written by some older path — where
// {fulfilment: null} would miss the empty string and {$exists: false} would
// miss both of the others.
func fulfilmentClause(fs []Fulfilment) (bson.E, bool) {
	if len(fs) == 0 {
		return bson.E{}, false
	}

	wantNone := false
	named := make(bson.A, 0, len(fs))
	for _, f := range fs {
		if f == FulfilmentNone {
			wantNone = true
			continue
		}
		named = append(named, string(f))
	}

	stored := make(bson.A, len(namedFulfilments))
	for i, f := range namedFulfilments {
		stored[i] = string(f)
	}

	switch {
	case wantNone && len(named) > 0:
		return bson.E{Key: opOr, Value: bson.A{
			bson.D{{Key: fieldFulfilment, Value: bson.D{{Key: opNin, Value: stored}}}},
			bson.D{{Key: fieldFulfilment, Value: bson.D{{Key: opIn, Value: named}}}},
		}}, true
	case wantNone:
		return bson.E{Key: fieldFulfilment, Value: bson.D{{Key: opNin, Value: stored}}}, true
	default:
		return bson.E{Key: fieldFulfilment, Value: bson.D{{Key: opIn, Value: named}}}, true
	}
}
