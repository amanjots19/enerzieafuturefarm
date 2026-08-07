package cart_test

import (
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/enerzia/enerzia-be/internal/cart"
	"github.com/enerzia/enerzia-be/internal/catalogue"
	"github.com/enerzia/enerzia-be/internal/mongotest"
)

const cartsNS = "enerzia_test.carts"

func newCartRepo(t *testing.T) (*cart.Repository, *mongotest.Server) {
	t.Helper()

	fake := mongotest.Start(t)
	client, err := mongo.Connect(options.Client().ApplyURI(fake.URI()).SetTimeout(5 * time.Second))
	if err != nil {
		t.Fatalf("connect to fake: %v", err)
	}
	t.Cleanup(func() { _ = client.Disconnect(t.Context()) })

	return cart.NewRepository(client.Database("enerzia_test")), fake
}

const storedProduct = catalogue.ID("tablets-120")

func TestRepoGetReturnsTheStoredCart(t *testing.T) {
	repo, fake := newCartRepo(t)
	fake.Respond("find", mongotest.Cursor(cartsNS, bson.D{
		{Key: "_id", Value: testUser},
		{Key: "lines", Value: bson.A{
			bson.D{
				{Key: "productId", Value: string(storedProduct)},
				{Key: "qty", Value: 3},
			},
		}},
		{Key: "updatedAt", Value: time.Now()},
	}))

	got, err := repo.Get(t.Context(), testUser)
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if len(got.Lines) != 1 {
		t.Fatalf("%d lines, want 1", len(got.Lines))
	}
	if got.Lines[0].ProductID != storedProduct || got.Lines[0].Qty != 3 {
		t.Errorf("line = %+v", got.Lines[0])
	}

	req, _ := fake.LastRequest("find")
	if got := req.Doc.Lookup("find").StringValue(); got != "carts" {
		t.Errorf("collection = %q, want carts", got)
	}
}

func TestRepoGetOfAnAbsentCartIsEmptyNotAnError(t *testing.T) {
	// A shopper who has never added anything has no document. That is an
	// empty cart, not a 404.
	repo, fake := newCartRepo(t)
	fake.Respond("find", mongotest.Cursor(cartsNS))

	got, err := repo.Get(t.Context(), testUser)
	if err != nil {
		t.Fatalf("Get() error = %v, want nil for a shopper with no cart", err)
	}
	if got.UserID != testUser {
		t.Errorf("UserID = %v, want the caller's id", got.UserID)
	}
	if len(got.Lines) != 0 {
		t.Errorf("%d lines, want 0", len(got.Lines))
	}
}

func TestRepoGetWrapsErrors(t *testing.T) {
	repo, fake := newCartRepo(t)
	fake.Respond("find", mongotest.Fail("not authorized", 13))

	_, err := repo.Get(t.Context(), testUser)
	if err == nil {
		t.Fatal("Get() error = nil, want the database failure to surface")
	}
	if !strings.Contains(err.Error(), "cart: get") {
		t.Errorf("error = %q, want it wrapped", err)
	}
}

func TestRepoSaveUpsertsTheWholeLineSet(t *testing.T) {
	repo, fake := newCartRepo(t)

	err := repo.Save(t.Context(), cart.Cart{
		UserID:    testUser,
		Lines:     []cart.StoredLine{{ProductID: "powder-100g", Qty: 2}},
		UpdatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("Save() error = %v, want nil", err)
	}

	req, ok := fake.LastRequest("update")
	if !ok {
		t.Fatal("no update command was sent")
	}
	updates := req.Sequence("updates")
	if len(updates) == 0 {
		t.Fatal("no update entry")
	}
	// Upsert, so a shopper's first add does not need a separate create path.
	if upsert, err := updates[0].LookupErr("upsert"); err != nil || !upsert.Boolean() {
		t.Error("Save must upsert")
	}

	set := updates[0].Lookup("u").Document().Lookup("$set").Document()
	if _, err := set.LookupErr("lines"); err != nil {
		t.Error("the update must set lines")
	}
	if _, err := set.LookupErr("updatedAt"); err != nil {
		t.Error("the update must stamp updatedAt")
	}
	// Prices must never be written into the cart (schema.md, decision 1).
	if s := updates[0].String(); strings.Contains(s, "unitPrice") || strings.Contains(s, "price") {
		t.Errorf("the cart write contains pricing: %s", s)
	}
}

func TestRepoSaveWritesAnEmptyArrayNotNull(t *testing.T) {
	// A nil slice would store `lines: null` and break the decode on read.
	repo, fake := newCartRepo(t)

	if err := repo.Save(t.Context(), cart.Cart{UserID: testUser, Lines: nil}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	req, _ := fake.LastRequest("update")
	lines := req.Sequence("updates")[0].Lookup("u").Document().Lookup("$set").Document().Lookup("lines")
	if lines.Type != bson.TypeArray {
		t.Errorf("lines stored as %v, want an array", lines.Type)
	}
}

func TestRepoSaveWrapsErrors(t *testing.T) {
	repo, fake := newCartRepo(t)
	fake.Respond("update", mongotest.Fail("not authorized", 13))

	err := repo.Save(t.Context(), cart.Cart{UserID: testUser})
	if err == nil {
		t.Fatal("Save() error = nil, want the database failure to surface")
	}
	if !strings.Contains(err.Error(), "cart: save") {
		t.Errorf("error = %q, want it wrapped", err)
	}
}

func TestRepoKeysTheCartByUserID(t *testing.T) {
	// One cart per shopper is enforced by the _id, not by a query filter.
	repo, fake := newCartRepo(t)

	if err := repo.Save(t.Context(), cart.Cart{UserID: testUser}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	req, _ := fake.LastRequest("update")
	filter := req.Sequence("updates")[0].Lookup("q").Document()
	id, err := filter.LookupErr("_id")
	if err != nil {
		t.Fatal("the update filter does not use _id")
	}
	if got, ok := id.ObjectIDOK(); !ok || got != testUser {
		t.Errorf("_id = %v, want the user id %v", id, testUser)
	}
}
