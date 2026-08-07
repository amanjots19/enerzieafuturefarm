package catalogue_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/enerzia/enerzia-be/internal/catalogue"
	"github.com/enerzia/enerzia-be/internal/mongotest"
)

const (
	productsNS = "enerzia_test.products"
	contentNS  = "enerzia_test.content"
)

// newRepo wires a repository to a fake MongoDB. The fake stores nothing, so
// tests declare the reply and then assert on the command that was sent.
func newRepo(t *testing.T) (*catalogue.Repository, *mongotest.Server) {
	t.Helper()

	fake := mongotest.Start(t)
	client, err := mongo.Connect(options.Client().ApplyURI(fake.URI()).SetTimeout(5 * time.Second))
	if err != nil {
		t.Fatalf("connect to fake: %v", err)
	}
	t.Cleanup(func() { _ = client.Disconnect(t.Context()) })

	return catalogue.NewRepository(client.Database("enerzia_test")), fake
}

// productDoc is the stored shape of a sellable product, per schema.md.
func productDoc(id, family, form, name string, price int64, stock int) bson.D {
	return bson.D{
		{Key: "_id", Value: id},
		{Key: "family", Value: family},
		{Key: "form", Value: form},
		{Key: "name", Value: name},
		{Key: "stat", Value: "stat"},
		{Key: "stat2", Value: "stat2"},
		{Key: "mrp", Value: int64(50000)},
		{Key: "price", Value: price},
		{Key: "stock", Value: stock},
		{Key: "position", Value: 0},
		{Key: "active", Value: true},
	}
}

// findRequestOn returns the find command issued against a collection.
func findRequestOn(t *testing.T, fake *mongotest.Server, collection string) bson.Raw {
	t.Helper()
	for _, r := range fake.Requests() {
		if r.Command == "find" && r.Doc.Lookup("find").StringValue() == collection {
			return r.Doc
		}
	}
	t.Fatalf("no find issued against %q", collection)
	return nil
}

/* ----------------------------------------------------------------- List */

func TestListReturnsEverySellableProduct(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Cursor(productsNS,
		productDoc("powder-100g", "powder", "Powder", "Pure Spirulina Powder — 100 g", 20000, 40),
		productDoc("tablets-120", "tablets", "Tablets", "Spirulina Tablets — 120 tabs", 38000, 10),
	))

	got, err := repo.List(t.Context(), "", false)
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if len(got) != 2 {
		t.Fatalf("List() returned %d products, want 2", len(got))
	}
	// Paise must survive the round trip as int64, not become a float.
	if got[0].Price != 20000 || got[0].Stock != 40 {
		t.Errorf("first product = %+v", got[0])
	}
	if got[1].Family != "tablets" {
		t.Errorf("family = %q, want tablets", got[1].Family)
	}
}

func TestListExcludesRetiredProducts(t *testing.T) {
	// A retired product must not appear in the grid, even though it stays
	// resolvable by id for old orders.
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Cursor(productsNS))

	if _, err := repo.List(t.Context(), "", false); err != nil {
		t.Fatalf("List() error = %v", err)
	}

	filter := findRequestOn(t, fake, "products").Lookup("filter").Document()
	active, err := filter.LookupErr("active")
	if err != nil || !active.Boolean() {
		t.Error("List must filter on active:true")
	}
}

func TestListFiltersByForm(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Cursor(productsNS,
		productDoc("powder-100g", "powder", "Powder", "Powder 100 g", 20000, 5),
		productDoc("refill-250g", "refill", "Powder", "Refill 250 g", 42000, 5),
	))

	got, err := repo.List(t.Context(), catalogue.FormPowder, true)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 2 {
		t.Errorf("List() returned %d products, want 2", len(got))
	}

	filter := findRequestOn(t, fake, "products").Lookup("filter").Document()
	if form := filter.Lookup("form").StringValue(); form != "Powder" {
		t.Errorf("filter.form = %q, want Powder", form)
	}
}

func TestListSortsForAStableGrid(t *testing.T) {
	// Without an explicit sort the grid could reshuffle between requests, and
	// siblings would not sit together.
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Cursor(productsNS))

	if _, err := repo.List(t.Context(), "", false); err != nil {
		t.Fatalf("List() error = %v", err)
	}

	sortVal, err := findRequestOn(t, fake, "products").LookupErr("sort")
	if err != nil {
		t.Fatalf("find carried no sort: %v", err)
	}
	if got := sortVal.Document().Lookup("position").AsInt64(); got != 1 {
		t.Errorf("sort.position = %d, want 1", got)
	}
}

func TestListReturnsEmptyNotNil(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Cursor(productsNS))

	got, err := repo.List(t.Context(), catalogue.FormBundle, true)
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("List() = %v, want an empty non-nil slice", got)
	}
}

func TestListWrapsDatabaseErrors(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Fail("not authorized", 13))

	_, err := repo.List(t.Context(), "", false)
	if err == nil {
		t.Fatal("List() error = nil, want the database failure to surface")
	}
	if !strings.Contains(err.Error(), "catalogue: list products") {
		t.Errorf("error = %q, want it wrapped with context", err)
	}
}

func TestListWrapsDecodeErrors(t *testing.T) {
	// A stored document whose shape has drifted must surface as an error, not
	// as a silently zeroed product with a price of 0.
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Cursor(productsNS, bson.D{
		{Key: "_id", Value: "powder-100g"},
		{Key: "price", Value: "not-a-number"},
	}))

	_, err := repo.List(t.Context(), "", false)
	if err == nil {
		t.Fatal("List() error = nil, want a decode failure")
	}
	if !strings.Contains(err.Error(), "catalogue: decode products") {
		t.Errorf("error = %q, want it wrapped with decode context", err)
	}
}

/* ------------------------------------------------------------- Siblings */

func TestSiblingsExcludesTheProductItself(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Cursor(productsNS,
		productDoc("tablets-60", "tablets", "Tablets", "60 tabs", 20000, 5),
	))

	got, err := repo.Siblings(t.Context(), "tablets", "tablets-120")
	if err != nil {
		t.Fatalf("Siblings() error = %v, want nil", err)
	}
	if len(got) != 1 {
		t.Fatalf("%d siblings, want 1", len(got))
	}

	filter := findRequestOn(t, fake, "products").Lookup("filter").Document()
	if fam := filter.Lookup("family").StringValue(); fam != "tablets" {
		t.Errorf("filter.family = %q, want tablets", fam)
	}
	ne, err := filter.LookupErr("_id")
	if err != nil {
		t.Fatal("Siblings must exclude the product itself")
	}
	if got := ne.Document().Lookup("$ne").StringValue(); got != "tablets-120" {
		t.Errorf("$ne = %q, want tablets-120", got)
	}
}

func TestSiblingsWrapsErrors(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Fail("not authorized", 13))

	if _, err := repo.Siblings(t.Context(), "tablets", "tablets-120"); err == nil {
		t.Error("Siblings() error = nil, want the failure to surface")
	}
}

/* ------------------------------------------------------------------ Get */

func TestGetReturnsTheProduct(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Cursor(productsNS,
		productDoc("tablets-120", "tablets", "Tablets", "120 tabs", 38000, 7),
	))

	got, err := repo.Get(t.Context(), "tablets-120")
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if got.ID != "tablets-120" || got.Stock != 7 {
		t.Errorf("product = %+v", got)
	}

	filter := findRequestOn(t, fake, "products").Lookup("filter").Document()
	if id := filter.Lookup("_id").StringValue(); id != "tablets-120" {
		t.Errorf("filter._id = %q, want tablets-120", id)
	}
}

func TestGetReportsNotFoundForAnUnknownID(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Cursor(productsNS))

	_, err := repo.Get(t.Context(), "capsules-10")
	if !errors.Is(err, catalogue.ErrProductNotFound) {
		t.Fatalf("Get() error = %v, want ErrProductNotFound", err)
	}
	// The handler maps this to a 404, so the id must survive for the log.
	if !strings.Contains(err.Error(), "capsules-10") {
		t.Errorf("error = %q, want it to name the missing id", err)
	}
}

func TestGetWrapsDatabaseErrors(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Fail("connection reset", 6))

	_, err := repo.Get(t.Context(), "powder-100g")
	if err == nil || errors.Is(err, catalogue.ErrProductNotFound) {
		t.Fatalf("Get() error = %v, want a wrapped database failure", err)
	}
}

/* ----------------------------------------------------- ProductsByID */

func TestProductsByIDReturnsAMap(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Cursor(productsNS,
		productDoc("powder-100g", "powder", "Powder", "Powder 100 g", 20000, 5),
		productDoc("tablets-120", "tablets", "Tablets", "120 tabs", 38000, 0),
	))

	got, err := repo.ProductsByID(t.Context(), []catalogue.ID{"powder-100g", "tablets-120"})
	if err != nil {
		t.Fatalf("ProductsByID() error = %v, want nil", err)
	}
	if len(got) != 2 {
		t.Fatalf("%d products, want 2", len(got))
	}
	// Sold out is derived, never read from a stored flag.
	if !got["tablets-120"].SoldOut() {
		t.Error("a zero-stock product should read as sold out")
	}
}

func TestProductsByIDWithNoIDsSkipsTheQuery(t *testing.T) {
	repo, fake := newRepo(t)

	got, err := repo.ProductsByID(t.Context(), nil)
	if err != nil {
		t.Fatalf("ProductsByID() error = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("%d products, want 0", len(got))
	}
	for _, r := range fake.Requests() {
		if r.Command == "find" {
			t.Error("an empty id list should not hit the database")
		}
	}
}

func TestProductsByIDWrapsErrors(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Fail("not authorized", 13))

	if _, err := repo.ProductsByID(t.Context(), []catalogue.ID{"powder-100g"}); err == nil {
		t.Error("ProductsByID() error = nil, want the failure to surface")
	}
}

/* ------------------------------------------------------------ TrustTiles */

func TestTrustTilesReturnsTheStoredTiles(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Cursor(contentNS, bson.D{
		{Key: "_id", Value: "trust"},
		{Key: "tiles", Value: bson.A{
			bson.D{{Key: "big", Value: "60%+"}, {Key: "body", Value: "Complete plant protein."}},
		}},
	}))

	got, err := repo.TrustTiles(t.Context())
	if err != nil {
		t.Fatalf("TrustTiles() error = %v, want nil", err)
	}
	if len(got) != 1 || got[0].Big != "60%+" {
		t.Errorf("TrustTiles() = %+v, want the stored tile", got)
	}
}

func TestTrustTilesIsEmptyRatherThanFailingWhenUnseeded(t *testing.T) {
	// The strip is decoration; a missing document must not take the shop down.
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Cursor(contentNS))

	got, err := repo.TrustTiles(t.Context())
	if err != nil {
		t.Fatalf("TrustTiles() error = %v, want nil for an unseeded database", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("TrustTiles() = %v, want an empty non-nil slice", got)
	}
}

func TestTrustTilesWrapsDatabaseErrors(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("find", mongotest.Fail("not authorized", 13))

	if _, err := repo.TrustTiles(t.Context()); err == nil {
		t.Fatal("TrustTiles() error = nil, want the database failure to surface")
	}
}

/* ----------------------------------------------------------------- stock */

func TestTakeStockGuardsInTheFilter(t *testing.T) {
	// The $gte clause is what makes the decrement safe under contention; an
	// unguarded $inc is atomic but happily oversells into negatives.
	repo, fake := newRepo(t)
	fake.Respond("update", mongotest.Reply(bson.D{
		{Key: "n", Value: 1}, {Key: "nModified", Value: 1}, {Key: "ok", Value: 1},
	}))

	if err := repo.TakeStock(t.Context(), "tablets-120", 3); err != nil {
		t.Fatalf("TakeStock() error = %v, want nil", err)
	}

	req, ok := fake.LastRequest("update")
	if !ok {
		t.Fatal("no update command was sent")
	}
	entry := req.Sequence("updates")[0]

	guard, err := entry.Lookup("q").Document().LookupErr("stock")
	if err != nil {
		t.Fatal("the filter has no stock guard; this would oversell")
	}
	if got := guard.Document().Lookup("$gte").AsInt64(); got != 3 {
		t.Errorf("guard $gte = %d, want 3", got)
	}
	if got := entry.Lookup("u").Document().Lookup("$inc").Document().Lookup("stock").AsInt64(); got != -3 {
		t.Errorf("$inc.stock = %d, want -3", got)
	}
}

func TestTakeStockReportsOutOfStockWhenNothingMatched(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("update", mongotest.Reply(bson.D{
		{Key: "n", Value: 0}, {Key: "nModified", Value: 0}, {Key: "ok", Value: 1},
	}))

	err := repo.TakeStock(t.Context(), "tablets-120", 1)
	if !errors.Is(err, catalogue.ErrOutOfStock) {
		t.Errorf("TakeStock() error = %v, want ErrOutOfStock", err)
	}
}

func TestTakeStockRejectsNonPositiveQuantities(t *testing.T) {
	repo, fake := newRepo(t)

	for _, qty := range []int{0, -1} {
		if err := repo.TakeStock(t.Context(), "tablets-120", qty); err == nil {
			t.Errorf("TakeStock(%d) error = nil, want a rejection", qty)
		}
	}
	for _, r := range fake.Requests() {
		if r.Command == "update" {
			t.Fatal("a nonsensical quantity reached the database")
		}
	}
}

func TestTakeStockWrapsErrors(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("update", mongotest.Fail("not authorized", 13))

	err := repo.TakeStock(t.Context(), "tablets-120", 1)
	if err == nil || errors.Is(err, catalogue.ErrOutOfStock) {
		t.Fatalf("TakeStock() error = %v, want a wrapped database failure", err)
	}
}

func TestReturnStockAddsUnitsBack(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("update", mongotest.Reply(bson.D{
		{Key: "n", Value: 1}, {Key: "nModified", Value: 1}, {Key: "ok", Value: 1},
	}))

	if err := repo.ReturnStock(t.Context(), "tablets-120", 2); err != nil {
		t.Fatalf("ReturnStock() error = %v, want nil", err)
	}

	req, _ := fake.LastRequest("update")
	inc := req.Sequence("updates")[0].Lookup("u").Document().Lookup("$inc").Document()
	if got := inc.Lookup("stock").AsInt64(); got != 2 {
		t.Errorf("$inc.stock = %d, want +2", got)
	}
}

func TestReturnStockRejectsNonPositiveQuantities(t *testing.T) {
	repo, _ := newRepo(t)

	if err := repo.ReturnStock(t.Context(), "tablets-120", 0); err == nil {
		t.Error("ReturnStock(0) error = nil, want a rejection")
	}
}

func TestReturnStockWrapsErrors(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("update", mongotest.Fail("not authorized", 13))

	if err := repo.ReturnStock(t.Context(), "tablets-120", 1); err == nil {
		t.Error("ReturnStock() error = nil, want the failure to surface")
	}
}

/* ------------------------------------------------------------------ Seed */

func TestSeedUpsertsEveryProductAndTheTrustTiles(t *testing.T) {
	repo, fake := newRepo(t)

	if err := repo.Seed(t.Context(), catalogue.SeedProducts(), catalogue.SeedTrustTiles()); err != nil {
		t.Fatalf("Seed() error = %v, want nil", err)
	}

	updates := 0
	for _, r := range fake.Requests() {
		if r.Command == "update" {
			updates++
		}
	}
	// Nine products plus the trust singleton.
	if updates != 10 {
		t.Errorf("sent %d update commands, want 10 (9 products + trust)", updates)
	}
}

func TestSeedPreservesStockOnReSeed(t *testing.T) {
	// A re-seed must refresh prices without resetting a live inventory count.
	repo, fake := newRepo(t)

	if err := repo.Seed(t.Context(), catalogue.SeedProducts()[:1], nil); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	// The trust singleton is written last, so pick the product write.
	var entry bson.Raw
	for _, r := range fake.Requests() {
		if r.Command != "update" {
			continue
		}
		q := r.Sequence("updates")[0].Lookup("q").Document()
		if q.Lookup("_id").StringValue() != "trust" {
			entry = r.Sequence("updates")[0]
			break
		}
	}
	if entry == nil {
		t.Fatal("no product update command was sent")
	}

	if upsert, err := entry.LookupErr("upsert"); err != nil || !upsert.Boolean() {
		t.Error("seed writes must be upserts, or a second run fails on existing ids")
	}
	set := entry.Lookup("u").Document().Lookup("$set").Document()
	if _, err := set.LookupErr("price"); err != nil {
		t.Error("a re-seed must refresh price")
	}
	if _, err := set.LookupErr("stock"); err == nil {
		t.Error("stock must NOT be under $set; a re-seed would reset inventory")
	}
	onInsert, err := entry.Lookup("u").Document().LookupErr("$setOnInsert")
	if err != nil {
		t.Fatal("stock is not under $setOnInsert")
	}
	if _, err := onInsert.Document().LookupErr("stock"); err != nil {
		t.Error("$setOnInsert does not carry stock")
	}
}

func TestSeedRejectsInvalidProductsBeforeWriting(t *testing.T) {
	// A seed run must not be how bad pricing reaches the database.
	repo, fake := newRepo(t)

	bad := catalogue.SeedProducts()
	bad[0].Price = bad[0].MRP + 1

	err := repo.Seed(t.Context(), bad, nil)
	if err == nil {
		t.Fatal("Seed() error = nil, want validation to reject the product")
	}
	if !strings.Contains(err.Error(), "exceeds mrp") {
		t.Errorf("error = %q, want the validation message", err)
	}
	for _, r := range fake.Requests() {
		if r.Command == "update" {
			t.Fatal("Seed() wrote to the database despite invalid input")
		}
	}
}

func TestSeedWrapsDatabaseErrors(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("update", mongotest.Fail("not authorized", 13))

	err := repo.Seed(t.Context(), catalogue.SeedProducts()[:1], nil)
	if err == nil {
		t.Fatal("Seed() error = nil, want the database failure to surface")
	}
	if !strings.Contains(err.Error(), "seed product") {
		t.Errorf("error = %q, want it to name the failing step", err)
	}
}

func TestSeedWrapsTrustTileErrors(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("update", mongotest.Fail("disk full", 14031))

	err := repo.Seed(t.Context(), nil, catalogue.SeedTrustTiles())
	if err == nil {
		t.Fatal("Seed() error = nil, want the database failure to surface")
	}
	if !strings.Contains(err.Error(), "seed trust tiles") {
		t.Errorf("error = %q, want it to name the failing step", err)
	}
}

/* --------------------------------------------------------- EnsureIndexes */

func TestEnsureIndexesCreatesBothGridIndexes(t *testing.T) {
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

	var sawForm, sawFamily bool
	for _, spec := range specs {
		keys := spec.Document().Lookup("key").Document()
		if _, err := keys.LookupErr("form"); err == nil {
			sawForm = true
		}
		if _, err := keys.LookupErr("family"); err == nil {
			sawFamily = true
		}
	}
	if !sawForm {
		t.Error("missing the form index that backs the grid filter")
	}
	if !sawFamily {
		t.Error("missing the family index that backs sibling lookups")
	}
}

func TestEnsureIndexesWrapsDatabaseErrors(t *testing.T) {
	repo, fake := newRepo(t)
	fake.Respond("createIndexes", mongotest.Fail("not authorized", 13))

	err := repo.EnsureIndexes(t.Context())
	if err == nil {
		t.Fatal("EnsureIndexes() error = nil, want the failure to surface")
	}
	if !strings.Contains(err.Error(), "ensure products indexes") {
		t.Errorf("error = %q, want it wrapped with context", err)
	}
}
