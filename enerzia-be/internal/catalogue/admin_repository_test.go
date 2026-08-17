package catalogue_test

import (
	"errors"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/enerzia/enerzia-be/internal/catalogue"
	"github.com/enerzia/enerzia-be/internal/mongotest"
)

/* ----------------------------------------------------------------- ListAll */

func TestListAllReturnsEveryProductIncludingRetired(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Cursor(productsNS,
		productDoc("powder-100g", "powder", "Powder", "Powder 100 g", 20000, 40),
		productDoc("tablets-120", "tablets", "Tablets", "Tablets 120", 38000, 10),
	))

	got, err := repo.ListAll(t.Context())
	if err != nil {
		t.Fatalf("ListAll() error = %v, want nil", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListAll() returned %d products, want 2", len(got))
	}
}

func TestListAllDoesNotFilterByActive(t *testing.T) {
	// Unlike List, ListAll must not add an active:true filter so the console
	// can see and revive retired products.
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Cursor(productsNS))

	if _, err := repo.ListAll(t.Context()); err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}

	filter := findRequestOn(t, fake, "products").Lookup("filter").Document()
	if _, err := filter.LookupErr("active"); err == nil {
		t.Error("ListAll must NOT filter by active; the admin console needs retired products")
	}
}

func TestListAllWrapsDatabaseErrors(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Fail("not authorized", 13))

	_, err := repo.ListAll(t.Context())
	if err == nil {
		t.Fatal("ListAll() error = nil, want the database failure to surface")
	}
	if !strings.Contains(err.Error(), "catalogue:") {
		t.Errorf("error = %q, want it wrapped with context", err)
	}
}

/* ------------------------------------------------------------------ Create */

func TestCreateInsertsTheProduct(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("insert", mongotest.Reply(bson.D{
		{Key: "ok", Value: int32(1)},
		{Key: "n", Value: int32(1)},
	}))

	p := catalogue.Product{
		ID:     "tablets-120",
		Family: "tablets",
		Form:   catalogue.FormTablets,
		Name:   "Tablets 120",
		MRP:    47000,
		Price:  38000,
		Stock:  100,
		Active: true,
	}
	if err := repo.Create(t.Context(), p); err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}

	req, ok := fake.LastRequest("insert")
	if !ok {
		t.Fatal("no insert command was sent")
	}
	docs := req.Sequence("documents")
	if len(docs) != 1 {
		t.Fatalf("insert sent %d documents, want 1", len(docs))
	}
	if got := docs[0].Lookup("_id").StringValue(); got != "tablets-120" {
		t.Errorf("inserted _id = %q, want tablets-120", got)
	}
}

func TestCreateReturnsDuplicateProductOnDupKey(t *testing.T) {
	// A real MongoDB duplicate-key error arrives as ok:1 with writeErrors[].
	// mongotest.Reply is required here; mongotest.Fail would produce a
	// CommandError (ok:0) which the driver cannot classify by keyPattern.
	repo, fake := newRepo(t)
	fake.Respond("insert", mongotest.Reply(bson.D{
		{Key: "ok", Value: int32(1)},
		{Key: "n", Value: int32(0)},
		{Key: "writeErrors", Value: bson.A{bson.D{
			{Key: "index", Value: int32(0)},
			{Key: "code", Value: int32(11000)},
			{Key: "errmsg", Value: `E11000 duplicate key error collection: enerzia_test.products index: _id_ dup key: { _id: "tablets-120" }`},
			{Key: "keyPattern", Value: bson.D{{Key: "_id", Value: int32(1)}}},
		}}},
	}))

	err := repo.Create(t.Context(), catalogue.Product{ID: "tablets-120"})
	if !errors.Is(err, catalogue.ErrDuplicateProduct) {
		t.Errorf("Create() error = %v, want ErrDuplicateProduct", err)
	}
}

func TestCreateWrapsDatabaseErrors(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("insert", mongotest.Fail("not authorized", 13))

	err := repo.Create(t.Context(), catalogue.Product{ID: "tablets-120"})
	if err == nil || errors.Is(err, catalogue.ErrDuplicateProduct) {
		t.Fatalf("Create() error = %v, want a wrapped database failure", err)
	}
}

/* ------------------------------------------------------------------ Update */

func TestUpdateSetsEditableFieldsAndNotID(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("update", mongotest.Reply(bson.D{
		{Key: "n", Value: int32(1)},
		{Key: "nModified", Value: int32(1)},
		{Key: "ok", Value: int32(1)},
	}))

	p := catalogue.Product{
		ID:       "tablets-120",
		Family:   "tablets-updated",
		Form:     catalogue.FormTablets,
		Name:     "New Name",
		MRP:      50000,
		Price:    40000,
		Stock:    50,
		Active:   true,
		Position: 3,
	}
	if err := repo.Update(t.Context(), p); err != nil {
		t.Fatalf("Update() error = %v, want nil", err)
	}

	req, ok := fake.LastRequest("update")
	if !ok {
		t.Fatal("no update command was sent")
	}
	entry := req.Sequence("updates")[0]

	// The query filter must match by _id.
	if got := entry.Lookup("q").Document().Lookup("_id").StringValue(); got != "tablets-120" {
		t.Errorf("update filter._id = %q, want tablets-120", got)
	}

	set := entry.Lookup("u").Document().Lookup("$set").Document()

	// _id must never appear in $set — that would rewrite the join key.
	if _, err := set.LookupErr("_id"); err == nil {
		t.Error("$set must not contain _id; rewriting it orphans cart and order lines")
	}

	// Spot-check that editable fields are present.
	if got := set.Lookup("name").StringValue(); got != "New Name" {
		t.Errorf("$set.name = %q, want New Name", got)
	}
	if got := set.Lookup("family").StringValue(); got != "tablets-updated" {
		t.Errorf("$set.family = %q, want tablets-updated", got)
	}
	if got := set.Lookup("mrp").AsInt64(); got != 50000 {
		t.Errorf("$set.mrp = %d, want 50000", got)
	}
}

func TestUpdateReturnsNotFoundWhenNoDocumentMatched(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("update", mongotest.Reply(bson.D{
		{Key: "n", Value: int32(0)},
		{Key: "nModified", Value: int32(0)},
		{Key: "ok", Value: int32(1)},
	}))

	err := repo.Update(t.Context(), catalogue.Product{ID: "nonexistent"})
	if !errors.Is(err, catalogue.ErrProductNotFound) {
		t.Errorf("Update() error = %v, want ErrProductNotFound", err)
	}
}

func TestUpdateWrapsDatabaseErrors(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("update", mongotest.Fail("not authorized", 13))

	err := repo.Update(t.Context(), catalogue.Product{ID: "tablets-120"})
	if err == nil || errors.Is(err, catalogue.ErrProductNotFound) {
		t.Fatalf("Update() error = %v, want a wrapped database failure", err)
	}
}

/* ------------------------------------------------------------------ Retire */

func TestRetireSetsActiveFalseViaDollarSet(t *testing.T) {
	// Retire must $set {active:false}, never delete. Past orders must still be
	// explainable via the product document.
	repo, fake := newRepo(t)
	fake.Respond("update", mongotest.Reply(bson.D{
		{Key: "n", Value: int32(1)},
		{Key: "nModified", Value: int32(1)},
		{Key: "ok", Value: int32(1)},
	}))

	if err := repo.Retire(t.Context(), "tablets-120"); err != nil {
		t.Fatalf("Retire() error = %v, want nil", err)
	}

	req, ok := fake.LastRequest("update")
	if !ok {
		t.Fatal("no update command was sent")
	}
	entry := req.Sequence("updates")[0]

	// Must use $set not a replace or delete.
	set := entry.Lookup("u").Document().Lookup("$set").Document()
	if active, err := set.LookupErr("active"); err != nil {
		t.Error("Retire must $set {active: false}")
	} else if active.Boolean() {
		t.Error("Retire must set active to false, not true")
	}
}

func TestRetireDoesNotDeleteDocument(t *testing.T) {
	// The update operation must not be a delete — verify by asserting on the
	// command name.
	repo, fake := newRepo(t)
	fake.Respond("update", mongotest.Reply(bson.D{
		{Key: "n", Value: int32(1)},
		{Key: "nModified", Value: int32(0)},
		{Key: "ok", Value: int32(1)},
	}))

	if err := repo.Retire(t.Context(), "tablets-120"); err != nil {
		t.Fatalf("Retire() error = %v, want nil (already retired is also ok)", err)
	}

	for _, r := range fake.Requests() {
		if r.Command == "delete" {
			t.Error("Retire must not delete the document; past orders still reference it")
		}
	}
}

func TestRetireReturnsNotFoundWhenNoDocumentMatched(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("update", mongotest.Reply(bson.D{
		{Key: "n", Value: int32(0)},
		{Key: "nModified", Value: int32(0)},
		{Key: "ok", Value: int32(1)},
	}))

	err := repo.Retire(t.Context(), "nonexistent")
	if !errors.Is(err, catalogue.ErrProductNotFound) {
		t.Errorf("Retire() error = %v, want ErrProductNotFound", err)
	}
}

func TestRetireAlreadyRetiredProductIsNotAnError(t *testing.T) {
	// An already-retired product has active=false. $set {active:false} matches
	// the document (matchedCount=1) even when it makes no change (modifiedCount=0).
	// The caller asked for a state and it holds — not an error.
	repo, fake := newRepo(t)
	fake.Respond("update", mongotest.Reply(bson.D{
		{Key: "n", Value: int32(1)},
		{Key: "nModified", Value: int32(0)}, // no actual change — already retired
		{Key: "ok", Value: int32(1)},
	}))

	if err := repo.Retire(t.Context(), "tablets-120"); err != nil {
		t.Errorf("Retire() error = %v, want nil for an already-retired product", err)
	}
}

func TestRetireWrapsDatabaseErrors(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("update", mongotest.Fail("not authorized", 13))

	err := repo.Retire(t.Context(), "tablets-120")
	if err == nil || errors.Is(err, catalogue.ErrProductNotFound) {
		t.Fatalf("Retire() error = %v, want a wrapped database failure", err)
	}
}
