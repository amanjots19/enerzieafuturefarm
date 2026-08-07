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

const paymentEventsNS = "enerzia_test.payment_events"

func newEventsRepo(t *testing.T) (*order.PaymentEventsRepository, *mongotest.Server) {
	t.Helper()
	fake := mongotest.Start(t)
	client, err := mongo.Connect(options.Client().ApplyURI(fake.URI()).SetTimeout(5 * time.Second))
	if err != nil {
		t.Fatalf("connect to fake: %v", err)
	}
	t.Cleanup(func() { _ = client.Disconnect(t.Context()) })
	return order.NewPaymentEventsRepository(client.Database("enerzia_test")), fake
}

func TestInsertEventRecordsTheEvent(t *testing.T) {
	repo, fake := newEventsRepo(t)
	e := order.PaymentEvent{
		ID:                "evt_001",
		Event:             "payment.captured",
		RazorpayOrderID:   "order_rzp",
		RazorpayPaymentID: "pay_xxx",
		ReceivedAt:        time.Now().UTC().Truncate(time.Millisecond),
		Processed:         true,
		PayloadDigest:     "sha256:abc123",
	}
	if err := repo.InsertEvent(t.Context(), e); err != nil {
		t.Fatalf("InsertEvent() error = %v, want nil", err)
	}

	req, ok := fake.LastRequest("insert")
	if !ok {
		t.Fatal("no insert command was sent")
	}
	if got := req.Doc.Lookup("insert").StringValue(); got != "payment_events" {
		t.Errorf("insert collection = %q, want payment_events", got)
	}
	docs := req.Sequence("documents")
	if len(docs) != 1 {
		t.Fatalf("inserted %d documents, want 1", len(docs))
	}
	if got := docs[0].Lookup("_id").StringValue(); got != "evt_001" {
		t.Errorf("_id = %q, want evt_001", got)
	}
}

func TestInsertEventDuplicateEventID(t *testing.T) {
	repo, fake := newEventsRepo(t)
	fake.Respond("insert", dupKeyReply(
		`E11000 duplicate key error collection: enerzia_test.payment_events index: _id_ dup key: { _id: "evt_001" }`,
		bson.D{{Key: "_id", Value: int32(1)}},
	))

	e := order.PaymentEvent{ID: "evt_001", Event: "payment.captured"}
	err := repo.InsertEvent(t.Context(), e)
	if !errors.Is(err, order.ErrDuplicateEvent) {
		t.Errorf("InsertEvent() error = %v, want ErrDuplicateEvent", err)
	}
}

func TestInsertEventWrapsGenericErrors(t *testing.T) {
	repo, fake := newEventsRepo(t)
	fake.Respond("insert", mongotest.Fail("not authorized", 13))

	e := order.PaymentEvent{ID: "evt_001", Event: "payment.captured"}
	err := repo.InsertEvent(t.Context(), e)
	if err == nil {
		t.Fatal("InsertEvent() error = nil, want the database failure to surface")
	}
	if errors.Is(err, order.ErrDuplicateEvent) {
		t.Error("a generic error must not be mapped to ErrDuplicateEvent")
	}
}

/* ----------------------------------------------------------------- GetEvent */

func TestGetEventReturnsStoredEvent(t *testing.T) {
	// GetEvent must query by _id so it retrieves the exact document written by
	// the original insert — not a scan or a secondary-index lookup.
	repo, fake := newEventsRepo(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	fake.Respond("find", mongotest.Cursor(paymentEventsNS, bson.D{
		{Key: "_id", Value: "evt_001"},
		{Key: "event", Value: "payment.captured"},
		{Key: "razorpayOrderId", Value: "order_rzp"},
		{Key: "processed", Value: false},
		{Key: "receivedAt", Value: now},
		{Key: "payloadDigest", Value: "sha256:abc"},
	}))

	got, err := repo.GetEvent(t.Context(), "evt_001")
	if err != nil {
		t.Fatalf("GetEvent() error = %v, want nil", err)
	}
	if got.ID != "evt_001" {
		t.Errorf("ID = %q, want evt_001", got.ID)
	}
	if got.Processed {
		t.Error("Processed = true, want false (the stored value)")
	}

	req, _ := fake.LastRequest("find")
	filter := req.Doc.Lookup("filter").Document()
	if id := filter.Lookup("_id").StringValue(); id != "evt_001" {
		t.Errorf("filter._id = %q, want evt_001", id)
	}
}

func TestGetEventWrapsErrors(t *testing.T) {
	repo, fake := newEventsRepo(t)
	fake.Respond("find", mongotest.Fail("connection reset", 6))

	_, err := repo.GetEvent(t.Context(), "evt_001")
	if err == nil {
		t.Fatal("GetEvent() error = nil, want a wrapped database failure")
	}
	if !strings.Contains(err.Error(), "order: get payment event") {
		t.Errorf("error = %q, want it wrapped with context", err)
	}
}

/* -------------------------------------------------------- MarkEventProcessed */

func TestMarkEventProcessedSetsProcessedTrue(t *testing.T) {
	// The update must target the event by _id and set processed to true.
	// This is the step that allows subsequent duplicate deliveries (Processed:true)
	// to be safely short-circuited.
	repo, fake := newEventsRepo(t)
	fake.Respond("update", mongotest.Reply(bson.D{
		{Key: "n", Value: 1}, {Key: "nModified", Value: 1}, {Key: "ok", Value: 1},
	}))

	if err := repo.MarkEventProcessed(t.Context(), "evt_001"); err != nil {
		t.Fatalf("MarkEventProcessed() error = %v, want nil", err)
	}

	req, ok := fake.LastRequest("update")
	if !ok {
		t.Fatal("no update command was sent")
	}
	entry := req.Sequence("updates")[0]
	if id := entry.Lookup("q").Document().Lookup("_id").StringValue(); id != "evt_001" {
		t.Errorf("filter._id = %q, want evt_001", id)
	}
	set := entry.Lookup("u").Document().Lookup("$set").Document()
	if !set.Lookup("processed").Boolean() {
		t.Error("$set.processed must be true")
	}
}

func TestMarkEventProcessedWrapsErrors(t *testing.T) {
	repo, fake := newEventsRepo(t)
	fake.Respond("update", mongotest.Fail("not authorized", 13))

	err := repo.MarkEventProcessed(t.Context(), "evt_001")
	if err == nil {
		t.Fatal("MarkEventProcessed() error = nil, want the database failure to surface")
	}
	if !strings.Contains(err.Error(), "order: mark payment event processed") {
		t.Errorf("error = %q, want it wrapped with context", err)
	}
}
