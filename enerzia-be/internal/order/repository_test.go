package order_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/enerzia/enerzia-be/internal/mongotest"
	"github.com/enerzia/enerzia-be/internal/order"
)

const ordersNS = "enerzia_test.orders"

func newRepo(t *testing.T) (*order.Repository, *mongotest.Server) {
	t.Helper()
	fake := mongotest.Start(t)
	client, err := mongo.Connect(options.Client().ApplyURI(fake.URI()).SetTimeout(5 * time.Second))
	if err != nil {
		t.Fatalf("connect to fake: %v", err)
	}
	t.Cleanup(func() { _ = client.Disconnect(t.Context()) })
	return order.NewRepository(client.Database("enerzia_test")), fake
}

// orderDoc builds a minimal BSON document that decodes cleanly into Order.
// Pointer fields (placedAt, capturedAt, failure) are omitted so they decode
// to nil — that is correct for a pending_payment order.
func orderDoc(oid string, uid bson.ObjectID, status order.Status) bson.D {
	now := time.Now()
	return bson.D{
		{Key: "_id", Value: bson.NewObjectID()},
		{Key: "orderId", Value: oid},
		{Key: "userId", Value: uid},
		{Key: "status", Value: string(status)},
		{Key: "lines", Value: bson.A{}},
		{Key: "totals", Value: bson.D{}},
		{Key: "payment", Value: bson.D{
			{Key: "provider", Value: "razorpay"},
			{Key: "status", Value: "created"},
			{Key: "amount", Value: int64(38000)},
			{Key: "currency", Value: "INR"},
			{Key: "razorpayOrderId", Value: "order_abc"},
			{Key: "attempts", Value: int32(0)},
		}},
		{Key: "shippingAddress", Value: bson.D{}},
		{Key: "createdAt", Value: now},
		{Key: "expiresAt", Value: now.Add(15 * time.Minute)},
		{Key: "updatedAt", Value: now},
	}
}

// minOrder returns the smallest Order value that satisfies Create.
func minOrder(uid bson.ObjectID) order.Order {
	now := time.Now().UTC().Truncate(time.Millisecond)
	return order.Order{
		OrderID: "EFF-000001",
		UserID:  uid,
		Status:  order.StatusPendingPayment,
		Lines: []order.Line{{
			ProductID: "tablets-120",
			Name:      "Tabs",
			Form:      "tablets",
			UnitPrice: 38000,
			UnitMrp:   47000,
			Qty:       1,
			LineTotal: 38000,
		}},
		Totals: order.Totals{
			MRPTotal: 47000,
			Subtotal: 38000,
			Savings:  9000,
			Shipping: 0,
			Total:    38000,
		},
		Payment:   order.NewPendingPayment("order_abc", 38000),
		CreatedAt: now,
		ExpiresAt: now.Add(15 * time.Minute),
		UpdatedAt: now,
	}
}

/* ----------------------------------------------------------------- Create */

func TestCreateInsertsTheOrder(t *testing.T) {
	repo, fake := newRepo(t)

	uid := bson.NewObjectID()
	if err := repo.Create(t.Context(), minOrder(uid)); err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}

	req, ok := fake.LastRequest("insert")
	if !ok {
		t.Fatal("no insert command was sent")
	}
	if got := req.Doc.Lookup("insert").StringValue(); got != "orders" {
		t.Errorf("insert collection = %q, want orders", got)
	}
	docs := req.Sequence("documents")
	if len(docs) != 1 {
		t.Fatalf("inserted %d documents, want 1", len(docs))
	}
	if got := docs[0].Lookup("orderId").StringValue(); got != "EFF-000001" {
		t.Errorf("inserted orderId = %q, want EFF-000001", got)
	}
}

// dupKeyReply builds a mongotest.Reply that simulates a real MongoDB write
// error response (ok:1, writeErrors:[{...}]). This causes the driver to
// produce a mongo.WriteException rather than a CommandError, which is required
// for keyPattern-based duplicate-key classification.
func dupKeyReply(errmsg string, keyPattern bson.D) mongotest.CommandResult {
	return mongotest.Reply(bson.D{
		{Key: "ok", Value: int32(1)},
		{Key: "n", Value: int32(0)},
		{Key: "writeErrors", Value: bson.A{bson.D{
			{Key: "index", Value: int32(0)},
			{Key: "code", Value: int32(11000)},
			{Key: "errmsg", Value: errmsg},
			{Key: "keyPattern", Value: keyPattern},
		}}},
	})
}

func TestCreateDuplicateOrderID(t *testing.T) {
	// orderId_1 fires when the generator produces a collision. The only correct
	// response is ErrDuplicateOrderID so the service can retry — returning
	// ErrPendingOrderExists would terminate the request with a 409.
	//
	// keyPattern {orderId:1} is the structural identity of this index. The
	// classification reads that field, not the index name, so a future rename
	// of the index cannot silently break the retry logic.
	repo, fake := newRepo(t)
	fake.Respond("insert", dupKeyReply(
		`E11000 duplicate key error collection: enerzia_test.orders index: orderId_1 dup key: { orderId: "EFF-000001" }`,
		bson.D{{Key: "orderId", Value: int32(1)}},
	))

	err := repo.Create(t.Context(), minOrder(bson.NewObjectID()))
	if !errors.Is(err, order.ErrDuplicateOrderID) {
		t.Errorf("Create() error = %v, want ErrDuplicateOrderID", err)
	}
}

func TestCreatePendingOrderExists(t *testing.T) {
	// userId_1 (partial, pending_payment) fires when the shopper already holds
	// a reservation. The only correct response is ErrPendingOrderExists —
	// never ErrDuplicateOrderID, because a retry on that error would loop forever.
	//
	// keyPattern {userId:1} — the ONLY unique index whose key is userId — is
	// what the classification checks. userId_1_createdAt_-1 is non-unique and
	// can never produce E11000, so there is no ambiguity in "userId".
	repo, fake := newRepo(t)
	fake.Respond("insert", dupKeyReply(
		`E11000 duplicate key error collection: enerzia_test.orders index: userId_1 dup key: { userId: ObjectId('000000000000000000000001') }`,
		bson.D{{Key: "userId", Value: int32(1)}},
	))

	err := repo.Create(t.Context(), minOrder(bson.NewObjectID()))
	if !errors.Is(err, order.ErrPendingOrderExists) {
		t.Errorf("Create() error = %v, want ErrPendingOrderExists", err)
	}
}

func TestCreateUnexpectedDuplicateKey(t *testing.T) {
	// A payment.razorpayOrderId collision at Create time is unexpected.
	// keyPattern {"payment.razorpayOrderId": 1} contains neither "orderId" nor
	// "userId" as exact keys, so neither sentinel matches and the error
	// surfaces wrapped for the caller to inspect.
	repo, fake := newRepo(t)
	fake.Respond("insert", dupKeyReply(
		`E11000 duplicate key error collection: enerzia_test.orders index: payment.razorpayOrderId_1 dup key: { payment.razorpayOrderId: "order_abc" }`,
		bson.D{{Key: "payment.razorpayOrderId", Value: int32(1)}},
	))

	err := repo.Create(t.Context(), minOrder(bson.NewObjectID()))
	if err == nil {
		t.Fatal("Create() error = nil, want unexpected duplicate key error")
	}
	if errors.Is(err, order.ErrDuplicateOrderID) {
		t.Error("got ErrDuplicateOrderID, want the unexpected-dup-key wrapped error")
	}
	if errors.Is(err, order.ErrPendingOrderExists) {
		t.Error("got ErrPendingOrderExists, want the unexpected-dup-key wrapped error")
	}
	if !strings.Contains(err.Error(), "order: create") {
		t.Errorf("error = %q, want it wrapped with context", err)
	}
}

func TestCreateWrapsGenericErrors(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("insert", mongotest.Fail("not authorized", 13))

	err := repo.Create(t.Context(), minOrder(bson.NewObjectID()))
	if err == nil {
		t.Fatal("Create() error = nil, want the database failure to surface")
	}
	if !strings.Contains(err.Error(), "order: create") {
		t.Errorf("error = %q, want it wrapped with context", err)
	}
}

/* --------------------------------------------------------------- ByOrderID */

func TestByOrderIDFiltersOnBothFields(t *testing.T) {
	// The filter MUST carry both orderId and userId in a single query.
	// A two-step fetch (by orderId, then compare userId in Go) leaks whether
	// the order exists through timing; it also omits userId from any index path.
	repo, fake := newRepo(t)
	uid := bson.NewObjectID()
	fake.Respond("find", mongotest.Cursor(ordersNS, orderDoc("EFF-000001", uid, order.StatusPendingPayment)))

	got, err := repo.ByOrderID(t.Context(), uid, "EFF-000001")
	if err != nil {
		t.Fatalf("ByOrderID() error = %v, want nil", err)
	}
	if got.OrderID != "EFF-000001" {
		t.Errorf("OrderID = %q, want EFF-000001", got.OrderID)
	}

	req, _ := fake.LastRequest("find")
	filter := req.Doc.Lookup("filter").Document()
	if oid := filter.Lookup("orderId").StringValue(); oid != "EFF-000001" {
		t.Errorf("filter.orderId = %q, want EFF-000001", oid)
	}
	gotUID, err2 := filter.LookupErr("userId")
	if err2 != nil {
		t.Fatal("filter must include userId to prevent cross-user access")
	}
	if gotUID.ObjectID() != uid {
		t.Errorf("filter.userId = %v, want %v", gotUID.ObjectID(), uid)
	}
}

func TestByOrderIDReturnsNotFoundForMissingOrder(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Cursor(ordersNS)) // empty cursor

	_, err := repo.ByOrderID(t.Context(), bson.NewObjectID(), "EFF-000001")
	if !errors.Is(err, order.ErrOrderNotFound) {
		t.Errorf("ByOrderID() error = %v, want ErrOrderNotFound", err)
	}
}

func TestByOrderIDWrapsErrors(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Fail("connection reset", 6))

	_, err := repo.ByOrderID(t.Context(), bson.NewObjectID(), "EFF-000001")
	if err == nil || errors.Is(err, order.ErrOrderNotFound) {
		t.Fatalf("ByOrderID() error = %v, want a wrapped database failure", err)
	}
	if !strings.Contains(err.Error(), "order: by order id") {
		t.Errorf("error = %q, want it wrapped with context", err)
	}
}

/* ------------------------------------------------------------- ListForUser */

func TestListForUserReturnsPaidOrdersOnlyAndSortsByCreatedAt(t *testing.T) {
	// Only paid orders (placed, packed, shipped, delivered, cancelled) must be
	// returned via an inclusive $in filter. pending_payment, payment_failed, and
	// expired are excluded. The $in filter fails safe: a new status is excluded
	// until explicitly added, unlike a growing exclusion list.
	// Sorted on createdAt (not placedAt) so the query works for all statuses
	// without null scatter.
	repo, fake := newRepo(t)
	uid := bson.NewObjectID()
	fake.Respond("find", mongotest.Cursor(ordersNS, orderDoc("EFF-000001", uid, order.StatusPlaced)))

	orders, err := repo.ListForUser(t.Context(), uid)
	if err != nil {
		t.Fatalf("ListForUser() error = %v, want nil", err)
	}
	if len(orders) != 1 {
		t.Fatalf("ListForUser() returned %d orders, want 1", len(orders))
	}

	req, _ := fake.LastRequest("find")
	filter := req.Doc.Lookup("filter").Document()

	// userId must be in the filter.
	gotUID, err2 := filter.LookupErr("userId")
	if err2 != nil || gotUID.ObjectID() != uid {
		t.Errorf("filter.userId = %v (err=%v), want %v", gotUID, err2, uid)
	}

	// Status must use $in over the five paid statuses — not $ne or $nin.
	statusVal, err2 := filter.LookupErr("status")
	if err2 != nil {
		t.Fatal("filter must include a status clause to restrict to paid orders")
	}
	inArr, err2 := statusVal.Document().LookupErr("$in")
	if err2 != nil {
		t.Fatal("status filter must use $in (inclusive), not $ne or $nin")
	}
	vals, err2 := inArr.Array().Values()
	if err2 != nil {
		t.Fatalf("$in value is not an array: %v", err2)
	}
	wantStatuses := []string{"placed", "packed", "shipped", "delivered", "cancelled"}
	if len(vals) != len(wantStatuses) {
		t.Errorf("$in has %d statuses, want %d", len(vals), len(wantStatuses))
	}
	for i, want := range wantStatuses {
		if i >= len(vals) {
			break
		}
		if got := vals[i].StringValue(); got != want {
			t.Errorf("$in[%d] = %q, want %q", i, got, want)
		}
	}

	// Sort by createdAt descending.
	sortVal, err2 := req.Doc.LookupErr("sort")
	if err2 != nil {
		t.Fatal("find must carry a sort so the caller gets newest-first without a secondary pass")
	}
	if got := sortVal.Document().Lookup("createdAt").AsInt64(); got != -1 {
		t.Errorf("sort.createdAt = %d, want -1 (descending)", got)
	}
}

func TestByOrderIDReturnsPendingPaymentOrders(t *testing.T) {
	// ByOrderID must return orders regardless of status — including pending_payment.
	// ListForUser excludes unpaid orders; ByOrderID does not. The obvious mistake
	// is to "consistently" filter both and silently 404 an order that exists.
	repo, fake := newRepo(t)
	uid := bson.NewObjectID()
	fake.Respond("find", mongotest.Cursor(ordersNS, orderDoc("EFF-118531", uid, order.StatusPendingPayment)))

	got, err := repo.ByOrderID(t.Context(), uid, "EFF-118531")
	if err != nil {
		t.Fatalf("ByOrderID() error = %v, want nil for pending_payment order", err)
	}
	if got.OrderID != "EFF-118531" {
		t.Errorf("OrderID = %q, want EFF-118531", got.OrderID)
	}

	// The filter must NOT include any status restriction.
	req, _ := fake.LastRequest("find")
	filter := req.Doc.Lookup("filter").Document()
	if _, err2 := filter.LookupErr("status"); err2 == nil {
		t.Error("ByOrderID filter must NOT include a status field — it returns orders of any status")
	}
}

func TestListForUserReturnsEmptySliceNotNil(t *testing.T) {
	// An empty slice is a successful result, not an error; and a nil slice
	// would encode to JSON null rather than [], breaking the API contract.
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Cursor(ordersNS)) // empty cursor

	got, err := repo.ListForUser(t.Context(), bson.NewObjectID())
	if err != nil {
		t.Fatalf("ListForUser() error = %v, want nil for empty result", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("ListForUser() = %v, want non-nil empty slice", got)
	}
}

func TestListForUserWrapsQueryErrors(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Fail("not authorized", 13))

	_, err := repo.ListForUser(t.Context(), bson.NewObjectID())
	if err == nil {
		t.Fatal("ListForUser() error = nil, want the database failure to surface")
	}
	if !strings.Contains(err.Error(), "order: list for user") {
		t.Errorf("error = %q, want it wrapped with context", err)
	}
}

func TestListForUserWrapsDecodeErrors(t *testing.T) {
	// A document whose totals field has the wrong type triggers a decode error
	// in cursor.All. The error must propagate rather than silently returning a
	// zero-value order in the slice.
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Cursor(ordersNS, bson.D{
		{Key: "_id", Value: bson.NewObjectID()},
		{Key: "totals", Value: "not-a-document"},
	}))

	_, err := repo.ListForUser(t.Context(), bson.NewObjectID())
	if err == nil {
		t.Fatal("ListForUser() error = nil, want a decode failure to surface")
	}
	if !strings.Contains(err.Error(), "order: decode orders") {
		t.Errorf("error = %q, want it wrapped with decode context", err)
	}
}

/* -------------------------------------------- ByRazorpayOrderID */

func TestByRazorpayOrderIDFiltersOnPaymentField(t *testing.T) {
	// The filter must use payment.razorpayOrderId — not orderId or userId — so
	// the webhook can resolve the order without an auth token.
	repo, fake := newRepo(t)
	uid := bson.NewObjectID()
	fake.Respond("find", mongotest.Cursor(ordersNS, orderDoc("EFF-000001", uid, order.StatusPendingPayment)))

	got, err := repo.ByRazorpayOrderID(t.Context(), "order_abc")
	if err != nil {
		t.Fatalf("ByRazorpayOrderID() error = %v, want nil", err)
	}
	if got.OrderID != "EFF-000001" {
		t.Errorf("OrderID = %q, want EFF-000001", got.OrderID)
	}

	req, _ := fake.LastRequest("find")
	filter := req.Doc.Lookup("filter").Document()
	if rzpID := filter.Lookup("payment.razorpayOrderId").StringValue(); rzpID != "order_abc" {
		t.Errorf("filter[payment.razorpayOrderId] = %q, want order_abc", rzpID)
	}
	// Must NOT filter by userId — the webhook carries no auth token.
	if _, err2 := filter.LookupErr("userId"); err2 == nil {
		t.Error("filter must NOT include userId — the webhook has no auth token")
	}
}

func TestByRazorpayOrderIDReturnsNotFoundForMissingOrder(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Cursor(ordersNS)) // empty cursor

	_, err := repo.ByRazorpayOrderID(t.Context(), "order_abc")
	if !errors.Is(err, order.ErrOrderNotFound) {
		t.Errorf("ByRazorpayOrderID() error = %v, want ErrOrderNotFound", err)
	}
}

func TestByRazorpayOrderIDWrapsErrors(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Fail("connection reset", 6))

	_, err := repo.ByRazorpayOrderID(t.Context(), "order_abc")
	if err == nil || errors.Is(err, order.ErrOrderNotFound) {
		t.Fatalf("ByRazorpayOrderID() error = %v, want a wrapped database failure", err)
	}
	if !strings.Contains(err.Error(), "order: by razorpay order id") {
		t.Errorf("error = %q, want it wrapped with context", err)
	}
}

/* -------------------------------------------- MarkPaymentFailed */

func TestMarkPaymentFailedGuardsStatusInFilter(t *testing.T) {
	// Only orders in pending_payment can transition to payment_failed. The status
	// guard in the filter prevents a concurrent expiry or duplicate delivery from
	// winning the transition twice.
	repo, fake := newRepo(t)
	fake.Respond("update", mongotest.Reply(bson.D{
		{Key: "n", Value: 1}, {Key: "nModified", Value: 1}, {Key: "ok", Value: 1},
	}))

	now := time.Now().UTC().Truncate(time.Millisecond)
	payment := order.NewPendingPayment("order_abc", 38000)
	modified, err := repo.MarkPaymentFailed(t.Context(), "EFF-000001", payment, now)
	if err != nil {
		t.Fatalf("MarkPaymentFailed() error = %v, want nil", err)
	}
	if !modified {
		t.Error("MarkPaymentFailed() = false, want true when a document was modified")
	}

	req, ok := fake.LastRequest("update")
	if !ok {
		t.Fatal("no update command was sent")
	}
	entry := req.Sequence("updates")[0]
	filter := entry.Lookup("q").Document()

	if got := filter.Lookup("orderId").StringValue(); got != "EFF-000001" {
		t.Errorf("filter.orderId = %q, want EFF-000001", got)
	}
	statusGuard, err2 := filter.LookupErr("status")
	if err2 != nil {
		t.Fatal("MarkPaymentFailed filter must include a status guard")
	}
	if got := statusGuard.StringValue(); got != "pending_payment" {
		t.Errorf("filter.status = %q, want pending_payment", got)
	}

	set := entry.Lookup("u").Document().Lookup("$set").Document()
	if got := set.Lookup("status").StringValue(); got != "payment_failed" {
		t.Errorf("$set.status = %q, want payment_failed", got)
	}
	if _, err2 := set.LookupErr("payment"); err2 != nil {
		t.Error("$set must include the payment sub-document")
	}
	if got := set.Lookup("updatedAt").Time(); !got.Equal(now) {
		t.Errorf("$set.updatedAt = %v, want %v (the now parameter)", got, now)
	}
}

func TestMarkPaymentFailedReturnsFalseNotErrorWhenNothingModified(t *testing.T) {
	// zero nModified means the order was already transitioned by another path.
	repo, fake := newRepo(t)
	fake.Respond("update", mongotest.Reply(bson.D{
		{Key: "n", Value: 0}, {Key: "nModified", Value: 0}, {Key: "ok", Value: 1},
	}))

	now := time.Now()
	modified, err := repo.MarkPaymentFailed(t.Context(), "EFF-000001", order.NewPendingPayment("order_abc", 38000), now)
	if err != nil {
		t.Fatalf("MarkPaymentFailed() error = %v, want nil", err)
	}
	if modified {
		t.Error("MarkPaymentFailed() = true, want false when nothing was modified")
	}
}

func TestMarkPaymentFailedWrapsErrors(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("update", mongotest.Fail("not authorized", 13))

	now := time.Now()
	_, err := repo.MarkPaymentFailed(t.Context(), "EFF-000001", order.NewPendingPayment("order_abc", 38000), now)
	if err == nil {
		t.Fatal("MarkPaymentFailed() error = nil, want the database failure to surface")
	}
	if !strings.Contains(err.Error(), "order: mark payment failed") {
		t.Errorf("error = %q, want it wrapped with context", err)
	}
}

/* ------------------------------------------------------------- MarkPlaced */

func TestMarkPlacedGuardsStatusInFilter(t *testing.T) {
	// The status guard is the atomicity guarantee: whichever of the callback
	// and the webhook arrives first transitions the order; the second finds
	// status != "pending_payment" and is a safe no-op returning (false, nil).
	repo, fake := newRepo(t)
	fake.Respond("update", mongotest.Reply(bson.D{
		{Key: "n", Value: 1}, {Key: "nModified", Value: 1}, {Key: "ok", Value: 1},
	}))

	placedAt := time.Now().UTC().Truncate(time.Millisecond)
	updatedAt := placedAt.Add(time.Millisecond) // distinct from placedAt so the test catches a swap
	payment := order.NewPendingPayment("order_abc", 38000)
	modified, err := repo.MarkPlaced(t.Context(), "EFF-000001", payment, placedAt, updatedAt)
	if err != nil {
		t.Fatalf("MarkPlaced() error = %v, want nil", err)
	}
	if !modified {
		t.Error("MarkPlaced() = false, want true when a document was modified")
	}

	req, ok := fake.LastRequest("update")
	if !ok {
		t.Fatal("no update command was sent")
	}
	entry := req.Sequence("updates")[0]
	filter := entry.Lookup("q").Document()

	if got := filter.Lookup("orderId").StringValue(); got != "EFF-000001" {
		t.Errorf("filter.orderId = %q, want EFF-000001", got)
	}
	statusGuard, err2 := filter.LookupErr("status")
	if err2 != nil {
		t.Fatal("MarkPlaced filter must include a status guard; without it callback and webhook both write")
	}
	if got := statusGuard.StringValue(); got != "pending_payment" {
		t.Errorf("filter.status = %q, want pending_payment", got)
	}

	// The update must advance the order to placed.
	set := entry.Lookup("u").Document().Lookup("$set").Document()
	if got := set.Lookup("status").StringValue(); got != "placed" {
		t.Errorf("$set.status = %q, want placed", got)
	}
	if _, err2 := set.LookupErr("payment"); err2 != nil {
		t.Error("$set must include the payment sub-document")
	}
	if _, err2 := set.LookupErr("placedAt"); err2 != nil {
		t.Error("$set must include placedAt")
	}
	// updatedAt must carry exactly the now parameter — not time.Now() from inside
	// the repository, which would be non-deterministic and untestable.
	if got := set.Lookup("updatedAt").Time(); !got.Equal(updatedAt) {
		t.Errorf("$set.updatedAt = %v, want %v (the now parameter)", got, updatedAt)
	}
}

func TestMarkPlacedReturnsFalseNotErrorWhenNothingModified(t *testing.T) {
	// zero nModified means the order was already placed by the other path
	// (callback or webhook). That is a successful no-op, not an error.
	repo, fake := newRepo(t)
	fake.Respond("update", mongotest.Reply(bson.D{
		{Key: "n", Value: 0}, {Key: "nModified", Value: 0}, {Key: "ok", Value: 1},
	}))

	now := time.Now()
	modified, err := repo.MarkPlaced(t.Context(), "EFF-000001", order.NewPendingPayment("order_abc", 38000), now, now)
	if err != nil {
		t.Fatalf("MarkPlaced() error = %v, want nil", err)
	}
	if modified {
		t.Error("MarkPlaced() = true, want false when nothing was modified")
	}
}

func TestMarkPlacedWrapsErrors(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("update", mongotest.Fail("not authorized", 13))

	now := time.Now()
	_, err := repo.MarkPlaced(t.Context(), "EFF-000001", order.NewPendingPayment("order_abc", 38000), now, now)
	if err == nil {
		t.Fatal("MarkPlaced() error = nil, want the database failure to surface")
	}
	if !strings.Contains(err.Error(), "order: mark placed") {
		t.Errorf("error = %q, want it wrapped with context", err)
	}
}

/* ------------------------------------------------------- EnsureIndexes */

func TestEnsureIndexesCreatesAllSevenIndexes(t *testing.T) {
	repo, fake := newRepo(t)

	if err := repo.EnsureIndexes(t.Context()); err != nil {
		t.Fatalf("EnsureIndexes() error = %v, want nil", err)
	}

	req, ok := fake.LastRequest("createIndexes")
	if !ok {
		t.Fatal("no createIndexes command was sent")
	}
	specs, err := bson.Raw(req.Doc.Lookup("indexes").Array()).Values()
	if err != nil {
		t.Fatalf("reading index specs: %v", err)
	}
	if len(specs) != 7 {
		t.Errorf("got %d index specs, want 7", len(specs))
	}

	type indexInfo struct {
		unique           bool
		hasPartialFilter bool
		partialStatus    string // non-empty only for status-scoped partial indexes
	}

	byName := make(map[string]indexInfo, 7)
	for _, spec := range specs {
		doc := spec.Document()
		name := doc.Lookup("name").StringValue()
		info := indexInfo{}
		if u, err2 := doc.LookupErr("unique"); err2 == nil {
			info.unique = u.Boolean()
		}
		if pfe, err2 := doc.LookupErr("partialFilterExpression"); err2 == nil {
			info.hasPartialFilter = true
			if sv, err3 := pfe.Document().LookupErr("status"); err3 == nil {
				info.partialStatus = sv.StringValue()
			}
		}
		byName[name] = info
	}

	// Each row: (name, wantUnique, wantPartial, wantPartialStatus).
	// "" for wantPartialStatus means partial filter scoped to field-existence
	// (payment.razorpayOrderId_1 / payment.razorpayPaymentId_1), not status.
	checks := []struct {
		name              string
		wantUnique        bool
		wantPartial       bool
		wantPartialStatus string
	}{
		{"orderId_1", true, false, ""},
		{"userId_1_createdAt_-1", false, false, ""},
		{"payment.razorpayOrderId_1", true, true, ""},
		{"payment.razorpayPaymentId_1", true, true, ""},
		{"userId_1", true, true, "pending_payment"},
		{"status_1_expiresAt_1", false, true, "pending_payment"},
		// The admin order book. Not partial: it must serve every status,
		// including ?status=all.
		{"status_1_createdAt_-1", false, false, ""},
	}

	for _, c := range checks {
		info, ok := byName[c.name]
		if !ok {
			t.Errorf("index %q is missing from createIndexes", c.name)
			continue
		}
		if info.unique != c.wantUnique {
			t.Errorf("index %q: unique = %v, want %v", c.name, info.unique, c.wantUnique)
		}
		if info.hasPartialFilter != c.wantPartial {
			t.Errorf("index %q: hasPartialFilter = %v, want %v", c.name, info.hasPartialFilter, c.wantPartial)
		}
		if c.wantPartialStatus != "" && info.partialStatus != c.wantPartialStatus {
			t.Errorf("index %q: partial filter status = %q, want %q", c.name, info.partialStatus, c.wantPartialStatus)
		}
	}
}

func TestEnsureIndexesWrapsErrors(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("createIndexes", mongotest.Fail("not authorized", 13))

	err := repo.EnsureIndexes(t.Context())
	if err == nil {
		t.Fatal("EnsureIndexes() error = nil, want the failure to surface")
	}
	if !strings.Contains(err.Error(), "order: ensure indexes") {
		t.Errorf("error = %q, want it wrapped with context", err)
	}
}

/* ----------------------------------------------- ExpirePendingForUser */

func TestExpirePendingForUserTransitionsToExpiredAndReturnsOrder(t *testing.T) {
	// The repository must issue a findAndModify (findOneAndUpdate) with:
	//   filter: {userId: uid, status: "pending_payment"}
	//   update: {$set: {status: "expired", updatedAt: now}}
	//   returnDocument: "before" (so we get the order's lines for stock return)
	repo, fake := newRepo(t)
	uid := bson.NewObjectID()
	now := time.Now().UTC().Truncate(time.Millisecond)

	fake.Respond("findAndModify", mongotest.Reply(bson.D{
		{Key: "ok", Value: int32(1)},
		{Key: "value", Value: orderDoc("EFF-000001", uid, order.StatusPendingPayment)},
		{Key: "lastErrorObject", Value: bson.D{
			{Key: "n", Value: int32(1)},
			{Key: "updatedExisting", Value: true},
		}},
	}))

	got, found, err := repo.ExpirePendingForUser(t.Context(), uid, now)
	if err != nil {
		t.Fatalf("ExpirePendingForUser() error = %v, want nil", err)
	}
	if !found {
		t.Error("ExpirePendingForUser() found = false, want true when an order exists")
	}
	if got.OrderID != "EFF-000001" {
		t.Errorf("OrderID = %q, want EFF-000001", got.OrderID)
	}

	req, ok := fake.LastRequest("findAndModify")
	if !ok {
		t.Fatal("no findAndModify command was sent")
	}

	// The filter must scope to this user and only pending_payment orders.
	query := req.Doc.Lookup("query").Document()
	if gotUID := query.Lookup("userId").ObjectID(); gotUID != uid {
		t.Errorf("filter.userId = %v, want %v", gotUID, uid)
	}
	if s := query.Lookup("status").StringValue(); s != "pending_payment" {
		t.Errorf("filter.status = %q, want pending_payment", s)
	}

	// The update must set status to expired.
	set := req.Doc.Lookup("update").Document().Lookup("$set").Document()
	if s := set.Lookup("status").StringValue(); s != "expired" {
		t.Errorf("$set.status = %q, want expired", s)
	}
	if got := set.Lookup("updatedAt").Time(); !got.Equal(now) {
		t.Errorf("$set.updatedAt = %v, want %v", got, now)
	}
}

func TestExpirePendingForUserReturnsFalseWhenNoOrderExists(t *testing.T) {
	// A shopper with no pending_payment order is normal — not an error.
	repo, fake := newRepo(t)
	fake.Respond("findAndModify", mongotest.Reply(bson.D{
		{Key: "ok", Value: int32(1)},
		{Key: "value", Value: nil}, // MongoDB returns null value when nothing matched
		{Key: "lastErrorObject", Value: bson.D{{Key: "n", Value: int32(0)}}},
	}))

	_, found, err := repo.ExpirePendingForUser(t.Context(), bson.NewObjectID(), time.Now())
	if err != nil {
		t.Fatalf("ExpirePendingForUser() error = %v, want nil for absent order", err)
	}
	if found {
		t.Error("ExpirePendingForUser() found = true, want false for absent order")
	}
}

func TestExpirePendingForUserWrapsErrors(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("findAndModify", mongotest.Fail("not authorized", 13))

	_, _, err := repo.ExpirePendingForUser(t.Context(), bson.NewObjectID(), time.Now())
	if err == nil {
		t.Fatal("ExpirePendingForUser() error = nil, want the failure to surface")
	}
	if !strings.Contains(err.Error(), "order: expire pending for user") {
		t.Errorf("error = %q, want it wrapped with context", err)
	}
}

/* ----------------------------------------------- SetRazorpayOrderID */

func TestSetRazorpayOrderIDUpdatesTheField(t *testing.T) {
	// The update must target the order by orderId and set payment.razorpayOrderId.
	repo, fake := newRepo(t)
	fake.Respond("update", mongotest.Reply(bson.D{
		{Key: "n", Value: 1}, {Key: "nModified", Value: 1}, {Key: "ok", Value: 1},
	}))

	if err := repo.SetRazorpayOrderID(t.Context(), "EFF-000001", "order_rzp123"); err != nil {
		t.Fatalf("SetRazorpayOrderID() error = %v, want nil", err)
	}

	req, ok := fake.LastRequest("update")
	if !ok {
		t.Fatal("no update command was sent")
	}
	entry := req.Sequence("updates")[0]

	// Filter must be by orderId.
	if oid := entry.Lookup("q").Document().Lookup("orderId").StringValue(); oid != "EFF-000001" {
		t.Errorf("filter.orderId = %q, want EFF-000001", oid)
	}
	// $set must include the razorpay order id.
	set := entry.Lookup("u").Document().Lookup("$set").Document()
	if rzpID := set.Lookup("payment.razorpayOrderId").StringValue(); rzpID != "order_rzp123" {
		t.Errorf("$set[payment.razorpayOrderId] = %q, want order_rzp123", rzpID)
	}
}

func TestSetRazorpayOrderIDWrapsErrors(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("update", mongotest.Fail("not authorized", 13))

	err := repo.SetRazorpayOrderID(t.Context(), "EFF-000001", "order_rzp123")
	if err == nil {
		t.Fatal("SetRazorpayOrderID() error = nil, want the failure to surface")
	}
	if !strings.Contains(err.Error(), "order: set razorpay order id") {
		t.Errorf("error = %q, want it wrapped with context", err)
	}
}

/* ------------------------------------------------- FindExpiredPending */

func TestFindExpiredPendingFiltersOnStatusAndExpiresAt(t *testing.T) {
	// The sweeper relies on the status_1_expiresAt_1 partial index; the filter
	// must carry both fields so the index is used and not just a full scan.
	repo, fake := newRepo(t)
	now := time.Now()
	fake.Respond("find", mongotest.Cursor(ordersNS))

	_, err := repo.FindExpiredPending(t.Context(), now, 50)
	if err != nil {
		t.Fatalf("FindExpiredPending() error = %v, want nil", err)
	}

	req, ok := fake.LastRequest("find")
	if !ok {
		t.Fatal("no find command was sent")
	}
	filter := req.Doc.Lookup("filter").Document()
	if s := filter.Lookup("status").StringValue(); s != "pending_payment" {
		t.Errorf("filter.status = %q, want pending_payment", s)
	}
	expiresAt := filter.Lookup("expiresAt").Document()
	if _, err := expiresAt.LookupErr("$lt"); err != nil {
		t.Error("filter.expiresAt must carry a $lt operator (schema.md §orders sweeper snippet)")
	}
}

func TestFindExpiredPendingEmptyResultIsNotError(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Cursor(ordersNS))

	orders, err := repo.FindExpiredPending(t.Context(), time.Now(), 50)
	if err != nil {
		t.Fatalf("FindExpiredPending() error = %v, want nil", err)
	}
	if len(orders) != 0 {
		t.Errorf("len(orders) = %d, want 0", len(orders))
	}
}

func TestFindExpiredPendingWrapsErrors(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Fail("connection reset", 6))

	_, err := repo.FindExpiredPending(t.Context(), time.Now(), 50)
	if err == nil {
		t.Fatal("FindExpiredPending() error = nil, want the database failure to surface")
	}
	if !strings.Contains(err.Error(), "order: find expired pending") {
		t.Errorf("error = %q, want it wrapped with context", err)
	}
}

/* ------------------------------------------------- MarkExpired */

func TestMarkExpiredGuardsStatusAndSetsExpired(t *testing.T) {
	// The guard in the filter is the whole correctness guarantee: without
	// status: "pending_payment" a concurrent MarkPlaced would not prevent
	// the sweeper from expiring an already-placed order and releasing stock
	// that the customer's paid order now owns.
	repo, fake := newRepo(t)
	now := time.Now()
	fake.Respond("update", mongotest.Reply(bson.D{
		{Key: "n", Value: int32(1)}, {Key: "nModified", Value: int32(1)}, {Key: "ok", Value: int32(1)},
	}))

	modified, err := repo.MarkExpired(t.Context(), "EFF-000001", now)
	if err != nil {
		t.Fatalf("MarkExpired() error = %v, want nil", err)
	}
	if !modified {
		t.Error("modified = false, want true (nModified = 1)")
	}

	req, ok := fake.LastRequest("update")
	if !ok {
		t.Fatal("no update command was sent")
	}
	entry := req.Sequence("updates")[0]
	filter := entry.Lookup("q").Document()
	if s := filter.Lookup("status").StringValue(); s != "pending_payment" {
		t.Errorf("filter.status = %q, want pending_payment (guard must be in the filter)", s)
	}
	if id := filter.Lookup("orderId").StringValue(); id != "EFF-000001" {
		t.Errorf("filter.orderId = %q, want EFF-000001", id)
	}
	set := entry.Lookup("u").Document().Lookup("$set").Document()
	if s := set.Lookup("status").StringValue(); s != "expired" {
		t.Errorf("$set.status = %q, want expired", s)
	}
}

func TestMarkExpiredReturnsFalseWhenGuardRejects(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("update", mongotest.Reply(bson.D{
		{Key: "n", Value: int32(0)}, {Key: "nModified", Value: int32(0)}, {Key: "ok", Value: int32(1)},
	}))

	modified, err := repo.MarkExpired(t.Context(), "EFF-000001", time.Now())
	if err != nil {
		t.Fatalf("MarkExpired() error = %v, want nil", err)
	}
	if modified {
		t.Error("modified = true, want false (nModified = 0 means the guard rejected the write)")
	}
}

func TestMarkExpiredWrapsErrors(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("update", mongotest.Fail("not authorized", 13))

	_, err := repo.MarkExpired(t.Context(), "EFF-000001", time.Now())
	if err == nil {
		t.Fatal("MarkExpired() error = nil, want the database failure to surface")
	}
	if !strings.Contains(err.Error(), "order: mark expired") {
		t.Errorf("error = %q, want it wrapped with context", err)
	}
}
