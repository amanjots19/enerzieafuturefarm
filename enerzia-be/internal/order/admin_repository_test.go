package order_test

import (
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/enerzia/enerzia-be/internal/mongotest"
	"github.com/enerzia/enerzia-be/internal/order"
)

// strValues reads a BSON array of strings out of a filter clause.
func strValues(t *testing.T, v bson.RawValue) []string {
	t.Helper()
	vals, err := v.Array().Values()
	if err != nil {
		t.Fatalf("value is not an array: %v", err)
	}
	out := make([]string, len(vals))
	for i, x := range vals {
		out[i] = x.StringValue()
	}
	return out
}

func TestListAllSortsNewestFirstAndCapsThePage(t *testing.T) {
	repo, fake := newRepo(t)
	uid := bson.NewObjectID()
	fake.Respond("find", mongotest.Cursor(ordersNS, orderDoc("EFF-000001", uid, order.StatusPlaced)))

	orders, err := repo.ListAll(t.Context(), order.AdminFilter{
		Statuses: order.PaidStatuses(),
		Limit:    25,
	})
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}
	if len(orders) != 1 {
		t.Fatalf("ListAll() returned %d orders, want 1", len(orders))
	}

	req, _ := fake.LastRequest("find")

	// Newest first. Without the sort the cursor is offset-ordered, and the
	// cursor paging below would return rows in an arbitrary order.
	sortVal, err := req.Doc.LookupErr("sort")
	if err != nil {
		t.Fatal("find must carry a sort")
	}
	if got := sortVal.Document().Lookup("createdAt").AsInt64(); got != -1 {
		t.Errorf("sort.createdAt = %d, want -1 (descending)", got)
	}

	// The limit must reach the driver. Without it a busy shop's whole order
	// history is decoded into memory on every page.
	limVal, err := req.Doc.LookupErr("limit")
	if err != nil {
		t.Fatal("find must carry a limit")
	}
	if got := limVal.AsInt64(); got != 25 {
		t.Errorf("limit = %d, want 25", got)
	}
}

func TestListAllDefaultsTheLimitWhenUnset(t *testing.T) {
	// A zero Limit is a caller mistake, not a request for the whole collection.
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Cursor(ordersNS))

	if _, err := repo.ListAll(t.Context(), order.AdminFilter{}); err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}

	req, _ := fake.LastRequest("find")
	if got := req.Doc.Lookup("limit").AsInt64(); got != order.AdminListDefaultLimit {
		t.Errorf("limit = %d, want the default %d", got, order.AdminListDefaultLimit)
	}
}

func TestListAllStatusFilter(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Cursor(ordersNS))

	_, err := repo.ListAll(t.Context(), order.AdminFilter{
		Statuses: []order.Status{order.StatusPlaced, order.StatusCancelled},
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}

	req, _ := fake.LastRequest("find")
	filter := req.Doc.Lookup("filter").Document()
	statusVal, err := filter.LookupErr("status")
	if err != nil {
		t.Fatal("filter must carry a status clause")
	}
	got := strValues(t, statusVal.Document().Lookup("$in"))
	if len(got) != 2 || got[0] != "placed" || got[1] != "cancelled" {
		t.Errorf("status.$in = %v, want [placed cancelled]", got)
	}
}

func TestListAllOmitsTheStatusClauseWhenUnfiltered(t *testing.T) {
	// ?status=all must produce NO status clause. An empty $in would match
	// nothing and quietly show an empty order book instead of every order.
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Cursor(ordersNS))

	if _, err := repo.ListAll(t.Context(), order.AdminFilter{Limit: 10}); err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}

	req, _ := fake.LastRequest("find")
	filter := req.Doc.Lookup("filter").Document()
	if _, err := filter.LookupErr("status"); err == nil {
		t.Error("filter carries a status clause when none was asked for")
	}
	if _, err := filter.LookupErr("fulfilment"); err == nil {
		t.Error("filter carries a fulfilment clause when none was asked for")
	}
}

func TestListAllCursorUsesLessThanNotLessThanOrEqual(t *testing.T) {
	// Strictly less-than. With $lte the row the cursor points at is served
	// again as the first row of the next page, so an operator sees a duplicate
	// at every page boundary.
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Cursor(ordersNS))
	before := time.Date(2026, 8, 14, 9, 12, 4, 0, time.UTC)

	_, err := repo.ListAll(t.Context(), order.AdminFilter{Before: &before, Limit: 10})
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}

	req, _ := fake.LastRequest("find")
	filter := req.Doc.Lookup("filter").Document()
	createdAt, err := filter.LookupErr("createdAt")
	if err != nil {
		t.Fatal("filter must carry a createdAt cursor")
	}
	if _, err := createdAt.Document().LookupErr("$lt"); err != nil {
		t.Errorf("createdAt clause = %v, want $lt", createdAt.Document())
	}
	if _, err := createdAt.Document().LookupErr("$lte"); err == nil {
		t.Error("createdAt uses $lte, which re-serves the boundary row")
	}
}

func TestListAllFulfilmentFilters(t *testing.T) {
	// FulfilmentNone is the ABSENCE of the key, so it cannot be matched by
	// equality. $nin over the stored values catches all three shapes an
	// untouched order can have: missing, null, or an empty string.
	tests := []struct {
		name string
		in   []order.Fulfilment
		// assert inspects the fulfilment clause the repository built.
		assert func(t *testing.T, filter bson.Raw)
	}{
		{
			name: "named states use $in",
			in:   []order.Fulfilment{order.FulfilmentPacked, order.FulfilmentShipped},
			assert: func(t *testing.T, filter bson.Raw) {
				v, err := filter.LookupErr("fulfilment")
				if err != nil {
					t.Fatal("no fulfilment clause")
				}
				got := strValues(t, v.Document().Lookup("$in"))
				if len(got) != 2 || got[0] != "packed" || got[1] != "shipped" {
					t.Errorf("fulfilment.$in = %v, want [packed shipped]", got)
				}
			},
		},
		{
			name: "none alone uses $nin over the stored values",
			in:   []order.Fulfilment{order.FulfilmentNone},
			assert: func(t *testing.T, filter bson.Raw) {
				v, err := filter.LookupErr("fulfilment")
				if err != nil {
					t.Fatal("no fulfilment clause")
				}
				if _, err := v.Document().LookupErr("$exists"); err == nil {
					t.Error("used $exists, which misses a null or empty-string value")
				}
				got := strValues(t, v.Document().Lookup("$nin"))
				if len(got) != 3 {
					t.Errorf("fulfilment.$nin = %v, want all three stored values", got)
				}
			},
		},
		{
			name: "none mixed with named states becomes an $or",
			in:   []order.Fulfilment{order.FulfilmentNone, order.FulfilmentPacked},
			assert: func(t *testing.T, filter bson.Raw) {
				v, err := filter.LookupErr("$or")
				if err != nil {
					t.Fatal("mixing none with a named state must produce an $or")
				}
				arms, err := v.Array().Values()
				if err != nil || len(arms) != 2 {
					t.Fatalf("$or = %v, want two arms", v)
				}
				if _, err := arms[0].Document().Lookup("fulfilment").Document().LookupErr("$nin"); err != nil {
					t.Error("the first arm should match untouched orders with $nin")
				}
				if _, err := arms[1].Document().Lookup("fulfilment").Document().LookupErr("$in"); err != nil {
					t.Error("the second arm should match the named states with $in")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, fake := newRepo(t)
			fake.Respond("find", mongotest.Cursor(ordersNS))

			_, err := repo.ListAll(t.Context(), order.AdminFilter{Fulfilments: tt.in, Limit: 10})
			if err != nil {
				t.Fatalf("ListAll() error = %v", err)
			}
			req, _ := fake.LastRequest("find")
			tt.assert(t, req.Doc.Lookup("filter").Document())
		})
	}
}

func TestListAllReturnsAnEmptySliceNotNil(t *testing.T) {
	// [] encodes as "orders":[]; nil would encode as null and make every client
	// guard against a case that never means anything different.
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Cursor(ordersNS))

	orders, err := repo.ListAll(t.Context(), order.AdminFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}
	if orders == nil {
		t.Fatal("ListAll() returned nil, want an empty slice")
	}
	if len(orders) != 0 {
		t.Errorf("ListAll() returned %d orders, want 0", len(orders))
	}
}

func TestListAllSurfacesDriverFailures(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Fail("connection reset", 6))

	if _, err := repo.ListAll(t.Context(), order.AdminFilter{Limit: 10}); err == nil {
		t.Fatal("ListAll() returned nil error on a failed find")
	}
}

func TestPaidStatusesCannotBeMutatedByACaller(t *testing.T) {
	// Both the shopper's list and the admin default read this. A caller that
	// reordered or truncated the shared slice would silently change what counts
	// as a purchase for everyone.
	first := order.PaidStatuses()
	first[0] = "tampered"

	if second := order.PaidStatuses(); second[0] == "tampered" {
		t.Error("PaidStatuses() handed out the underlying slice")
	}
}

/* ------------------------------------------------------- ByOrderIDUnscoped */

func TestByOrderIDUnscopedDoesNotFilterOnUser(t *testing.T) {
	// The whole point of this method: the admin book must reach ANY shopper's
	// order. A stray userId in the filter would silently return 404 for every
	// order that is not the admin's own — which, since an admin has no user
	// document, is all of them.
	repo, fake := newRepo(t)
	uid := bson.NewObjectID()
	fake.Respond("find", mongotest.Cursor(ordersNS, orderDoc("EFF-000001", uid, order.StatusPlaced)))

	got, err := repo.ByOrderIDUnscoped(t.Context(), "EFF-000001")
	if err != nil {
		t.Fatalf("ByOrderIDUnscoped() error = %v", err)
	}
	if got.OrderID != "EFF-000001" {
		t.Errorf("orderId = %q, want EFF-000001", got.OrderID)
	}

	req, _ := fake.LastRequest("find")
	filter := req.Doc.Lookup("filter").Document()
	if v := filter.Lookup("orderId").StringValue(); v != "EFF-000001" {
		t.Errorf("filter.orderId = %q, want EFF-000001", v)
	}
	if _, err := filter.LookupErr("userId"); err == nil {
		t.Error("filter carries a userId — this lookup must be unscoped")
	}
}

func TestByOrderIDUnscopedNotFound(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Cursor(ordersNS)) // empty cursor

	_, err := repo.ByOrderIDUnscoped(t.Context(), "EFF-000001")
	if !errors.Is(err, order.ErrOrderNotFound) {
		t.Errorf("error = %v, want ErrOrderNotFound", err)
	}
}

func TestByOrderIDUnscopedSurfacesDriverFailures(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Fail("connection reset", 6))

	_, err := repo.ByOrderIDUnscoped(t.Context(), "EFF-000001")
	if err == nil {
		t.Fatal("error = nil, want the driver failure to surface")
	}
	if errors.Is(err, order.ErrOrderNotFound) {
		t.Error("a driver failure was reported as not-found")
	}
}

/* ------------------------------------------------------------ SetFulfilment */

func TestSetFulfilmentGuardsOnTheCurrentState(t *testing.T) {
	// The filter must carry the state the caller believed the order was in.
	// Without it two operators clicking at once both succeed and a step is
	// skipped — a parcel goes packed -> shipped with nobody marking it in
	// transit.
	repo, fake := newRepo(t)
	fake.Respond("update", mongotest.Reply(bson.D{
		{Key: "ok", Value: int32(1)}, {Key: "n", Value: int32(1)}, {Key: "nModified", Value: int32(1)},
	}))
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)

	ok, err := repo.SetFulfilment(t.Context(), "EFF-000001",
		order.FulfilmentPacked, order.FulfilmentInTransit, now)
	if err != nil {
		t.Fatalf("SetFulfilment() error = %v", err)
	}
	if !ok {
		t.Error("SetFulfilment() = false, want true when a document was modified")
	}

	req, _ := fake.LastRequest("update")
	upd := req.Sequence("updates")[0]
	filter := upd.Lookup("q").Document()

	if got := filter.Lookup("orderId").StringValue(); got != "EFF-000001" {
		t.Errorf("filter.orderId = %q", got)
	}
	if got := filter.Lookup("fulfilment").StringValue(); got != "packed" {
		t.Errorf("filter.fulfilment = %q, want the FROM state packed", got)
	}
	// An order that expired between the read and the write must not acquire
	// fulfilment progress.
	if got := filter.Lookup("status").StringValue(); got != "placed" {
		t.Errorf("filter.status = %q, want placed", got)
	}

	set := upd.Lookup("u").Document().Lookup("$set").Document()
	if got := set.Lookup("fulfilment").StringValue(); got != "in_transit" {
		t.Errorf("$set.fulfilment = %q", got)
	}
	// Only fulfilment and updatedAt. Never status, never totals, never payment.
	elems, _ := set.Elements()
	if len(elems) != 2 {
		t.Errorf("$set writes %d fields, want exactly fulfilment and updatedAt", len(elems))
	}
	for _, e := range elems {
		if k := e.Key(); k != "fulfilment" && k != "updatedAt" {
			t.Errorf("$set writes unexpected field %q", k)
		}
	}
}

func TestSetFulfilmentFromNoneMatchesAnAbsentKey(t *testing.T) {
	// FulfilmentNone is the absence of the key, so the guard cannot use
	// equality — {fulfilment: ""} would match no untouched order at all and
	// every first advance would report a phantom conflict.
	repo, fake := newRepo(t)
	fake.Respond("update", mongotest.Reply(bson.D{
		{Key: "ok", Value: int32(1)}, {Key: "n", Value: int32(1)}, {Key: "nModified", Value: int32(1)},
	}))

	_, err := repo.SetFulfilment(t.Context(), "EFF-000001",
		order.FulfilmentNone, order.FulfilmentPacked, time.Now())
	if err != nil {
		t.Fatalf("SetFulfilment() error = %v", err)
	}

	req, _ := fake.LastRequest("update")
	filter := req.Sequence("updates")[0].Lookup("q").Document()
	clause, err := filter.LookupErr("fulfilment")
	if err != nil {
		t.Fatal("no fulfilment guard in the filter")
	}
	got := strValues(t, clause.Document().Lookup("$nin"))
	if len(got) != 3 {
		t.Errorf("fulfilment.$nin = %v, want the three stored values", got)
	}
}

func TestSetFulfilmentReportsAGuardMiss(t *testing.T) {
	// nModified 0 means somebody else moved it first. That must surface as
	// false, not as a silent success.
	repo, fake := newRepo(t)
	fake.Respond("update", mongotest.Reply(bson.D{
		{Key: "ok", Value: int32(1)}, {Key: "n", Value: int32(0)}, {Key: "nModified", Value: int32(0)},
	}))

	ok, err := repo.SetFulfilment(t.Context(), "EFF-000001",
		order.FulfilmentPacked, order.FulfilmentInTransit, time.Now())
	if err != nil {
		t.Fatalf("SetFulfilment() error = %v", err)
	}
	if ok {
		t.Error("SetFulfilment() = true, want false when the guard matched nothing")
	}
}

func TestSetFulfilmentSurfacesDriverFailures(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("update", mongotest.Fail("not authorized", 13))

	if _, err := repo.SetFulfilment(t.Context(), "EFF-000001",
		order.FulfilmentPacked, order.FulfilmentInTransit, time.Now()); err == nil {
		t.Fatal("error = nil, want the driver failure to surface")
	}
}

/* ------------------------------------------------------- FillPaymentDetail */

// TestFillPaymentDetailOnlyTouchesAnEmptyMethod pins the guard. Razorpay's
// callback cannot know how the shopper paid and almost always wins the race to
// MarkPlaced, so the webhook's method was being discarded and every order was
// left unable to say how it was paid.
func TestFillPaymentDetailOnlyTouchesAnEmptyMethod(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("update", mongotest.Reply(bson.D{
		{Key: "ok", Value: int32(1)}, {Key: "n", Value: int32(1)}, {Key: "nModified", Value: int32(1)},
	}))

	ok, err := repo.FillPaymentDetail(t.Context(), "EFF-000001", order.Payment{
		Method: order.PaymentNetbanking, Label: "Netbanking", Bank: "HDFC",
	}, time.Now())
	if err != nil {
		t.Fatalf("FillPaymentDetail() error = %v", err)
	}
	if !ok {
		t.Error("FillPaymentDetail() = false, want true when a document was modified")
	}

	req, _ := fake.LastRequest("update")
	upd := req.Sequence("updates")[0]
	filter := upd.Lookup("q").Document()

	if got := filter.Lookup("orderId").StringValue(); got != "EFF-000001" {
		t.Errorf("filter.orderId = %q", got)
	}
	// Only an already-placed order. This must never resurrect an expired one.
	if got := filter.Lookup("status").StringValue(); got != "placed" {
		t.Errorf("filter.status = %q, want placed", got)
	}
	// The method must still be empty, so a redelivery cannot overwrite a value
	// already recorded correctly.
	guard, err := filter.LookupErr("payment.method")
	if err != nil {
		t.Fatal("no guard on payment.method — a redelivery could overwrite it")
	}
	vals, err := guard.Document().Lookup("$in").Array().Values()
	if err != nil || len(vals) != 2 {
		t.Errorf("payment.method guard = %v, want $in over null and empty string", guard)
	}

	set := upd.Lookup("u").Document().Lookup("$set").Document()
	if got := set.Lookup("payment.method").StringValue(); got != "netbanking" {
		t.Errorf("$set payment.method = %q", got)
	}
	if got := set.Lookup("payment.bank").StringValue(); got != "HDFC" {
		t.Errorf("$set payment.bank = %q", got)
	}

	// It must NOT touch anything that settles the order or the money. Writing
	// the whole payment sub-document here would clobber the callback's stored
	// signature and the captured amount.
	elems, _ := set.Elements()
	for _, e := range elems {
		switch e.Key() {
		case "status", "placedAt", "payment.amount", "payment.status",
			"payment.razorpaySignature", "payment.razorpayPaymentId", "payment":
			t.Errorf("$set writes %q, which this must never touch", e.Key())
		}
	}
}

func TestFillPaymentDetailReportsNothingToFill(t *testing.T) {
	// The normal case once an order already has its method: no document
	// matched, and that is not an error.
	repo, fake := newRepo(t)
	fake.Respond("update", mongotest.Reply(bson.D{
		{Key: "ok", Value: int32(1)}, {Key: "n", Value: int32(0)}, {Key: "nModified", Value: int32(0)},
	}))

	ok, err := repo.FillPaymentDetail(t.Context(), "EFF-000001",
		order.Payment{Method: order.PaymentUPI}, time.Now())
	if err != nil {
		t.Fatalf("FillPaymentDetail() error = %v", err)
	}
	if ok {
		t.Error("FillPaymentDetail() = true when nothing matched")
	}
}

func TestFillPaymentDetailSurfacesDriverFailures(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("update", mongotest.Fail("not authorized", 13))

	if _, err := repo.FillPaymentDetail(t.Context(), "EFF-000001",
		order.Payment{Method: order.PaymentUPI}, time.Now()); err == nil {
		t.Fatal("error = nil, want the driver failure to surface")
	}
}
