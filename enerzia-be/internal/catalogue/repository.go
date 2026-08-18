package catalogue

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Collection and field names, per schema.md.
const (
	productsCollection = "products"
	contentCollection  = "content"
	// trustContentID keys the singleton document holding the shop's trust
	// tiles inside the shared content collection.
	trustContentID = "trust"

	fieldID       = "_id"
	fieldForm     = "form"
	fieldFamily   = "family"
	fieldPosition = "position"
	fieldActive   = "active"
	fieldStock    = "stock"
	fieldImages   = "images"

	// opSet is the MongoDB $set update operator.
	opSet = "$set"
)

// ErrProductNotFound is returned when no product has the requested id.
var ErrProductNotFound = errors.New("catalogue: product not found")

// ErrOutOfStock is returned when a stock decrement finds too few units.
var ErrOutOfStock = errors.New("catalogue: out of stock")

// ErrDuplicateProduct is returned when Create finds a product with the same id.
var ErrDuplicateProduct = errors.New("catalogue: duplicate product id")

// Repository reads and seeds the catalogue.
type Repository struct {
	products *mongo.Collection
	content  *mongo.Collection
}

// NewRepository builds a repository over the given database.
func NewRepository(db *mongo.Database) *Repository {
	return &Repository{
		products: db.Collection(productsCollection),
		content:  db.Collection(contentCollection),
	}
}

type trustDocument struct {
	ID    string      `bson:"_id"`
	Tiles []TrustTile `bson:"tiles"`
}

// byPosition orders the grid so siblings sit together without the client
// sorting.
func byPosition() *options.FindOptionsBuilder {
	return options.Find().SetSort(bson.D{
		{Key: fieldPosition, Value: 1},
		{Key: fieldID, Value: 1},
	})
}

// List returns the sellable catalogue, optionally narrowed to one form.
// Retired products are excluded everywhere a shopper can see.
func (r *Repository) List(ctx context.Context, form Form, filtered bool) ([]Product, error) {
	filter := bson.D{{Key: fieldActive, Value: true}}
	if filtered {
		filter = append(filter, bson.E{Key: fieldForm, Value: string(form)})
	}
	return r.find(ctx, filter, "list products")
}

// Siblings returns the other active products in a family, so a detail page can
// offer the remaining sizes.
func (r *Repository) Siblings(ctx context.Context, family Family, exclude ID) ([]Product, error) {
	found, err := r.find(ctx, bson.D{
		{Key: fieldFamily, Value: string(family)},
		{Key: fieldActive, Value: true},
		{Key: fieldID, Value: bson.D{{Key: "$ne", Value: string(exclude)}}},
	}, "list siblings")
	if err != nil {
		return nil, err
	}
	return found, nil
}

func (r *Repository) find(ctx context.Context, filter bson.D, op string) ([]Product, error) {
	cursor, err := r.products.Find(ctx, filter, byPosition())
	if err != nil {
		return nil, fmt.Errorf("catalogue: %s: %w", op, err)
	}

	var products []Product
	if err := cursor.All(ctx, &products); err != nil {
		return nil, fmt.Errorf("catalogue: decode products: %w", err)
	}
	if products == nil {
		return []Product{}, nil
	}
	return products, nil
}

// Get returns one product, retired or not. A retired product stays resolvable
// so an old order can still be explained; callers that must not sell it check
// Active themselves.
func (r *Repository) Get(ctx context.Context, id ID) (Product, error) {
	var product Product
	err := r.products.FindOne(ctx, bson.D{{Key: fieldID, Value: string(id)}}).Decode(&product)

	switch {
	case errors.Is(err, mongo.ErrNoDocuments):
		return Product{}, fmt.Errorf("%w: %s", ErrProductNotFound, id)
	case err != nil:
		return Product{}, fmt.Errorf("catalogue: get product %q: %w", id, err)
	}
	return product, nil
}

// ProductsByID resolves the products a cart or order refers to, keyed by id.
// Retired products are included: a cart holding one must be able to explain it
// rather than silently dropping the line.
func (r *Repository) ProductsByID(ctx context.Context, ids []ID) (map[ID]Product, error) {
	out := make(map[ID]Product, len(ids))
	if len(ids) == 0 {
		return out, nil
	}

	raw := make([]string, 0, len(ids))
	for _, id := range ids {
		raw = append(raw, string(id))
	}

	found, err := r.find(ctx, bson.D{
		{Key: fieldID, Value: bson.D{{Key: "$in", Value: raw}}},
	}, "get products")
	if err != nil {
		return nil, err
	}
	for _, p := range found {
		out[p.ID] = p
	}
	return out, nil
}

// TrustTiles returns the statistics shown under the shop grid. A missing
// document yields an empty slice, not an error: the strip is decoration, and
// an unseeded database should not take the shop page down.
func (r *Repository) TrustTiles(ctx context.Context) ([]TrustTile, error) {
	var doc trustDocument
	err := r.content.FindOne(ctx, bson.D{{Key: fieldID, Value: trustContentID}}).Decode(&doc)

	switch {
	case errors.Is(err, mongo.ErrNoDocuments):
		return []TrustTile{}, nil
	case err != nil:
		return nil, fmt.Errorf("catalogue: get trust tiles: %w", err)
	}
	return doc.Tiles, nil
}

// TakeStock decrements a product's stock by qty, atomically.
//
// The `$gte` guard lives in the filter, not in a prior read: that is what makes
// it safe under contention. An unguarded $inc is still atomic but happily
// drives stock negative. ErrOutOfStock means someone else took the units
// between the caller's check and this write.
func (r *Repository) TakeStock(ctx context.Context, id ID, qty int) error {
	if qty <= 0 {
		return fmt.Errorf("catalogue: take stock: qty must be positive, got %d", qty)
	}

	res, err := r.products.UpdateOne(ctx,
		bson.D{
			{Key: fieldID, Value: string(id)},
			{Key: fieldStock, Value: bson.D{{Key: "$gte", Value: qty}}},
		},
		bson.D{{Key: "$inc", Value: bson.D{{Key: fieldStock, Value: -qty}}}},
	)
	if err != nil {
		return fmt.Errorf("catalogue: take stock: %w", err)
	}
	if res.ModifiedCount == 0 {
		return fmt.Errorf("%w: product %s", ErrOutOfStock, id)
	}
	return nil
}

// ReturnStock puts units back. It is the compensating write when a later step
// of order placement fails after some decrements have already landed.
func (r *Repository) ReturnStock(ctx context.Context, id ID, qty int) error {
	if qty <= 0 {
		return fmt.Errorf("catalogue: return stock: qty must be positive, got %d", qty)
	}
	if _, err := r.products.UpdateByID(ctx, string(id),
		bson.D{{Key: "$inc", Value: bson.D{{Key: fieldStock, Value: qty}}}},
	); err != nil {
		return fmt.Errorf("catalogue: return stock: %w", err)
	}
	return nil
}

// Seed upserts the supplied catalogue. It is idempotent: prices and copy are
// refreshed, but **stock is only set on insert** so a re-seed cannot reset a
// live inventory count.
// Seed creates any product that does not yet exist and LEAVES EXISTING
// PRODUCTS UNTOUCHED.
//
// The catalogue is maintained through the admin console: prices, copy, badges
// and the active flag are an administrator's to set. A seed that wrote over
// them would silently revert that work — re-pricing a product back to its
// original value, or un-retiring something deliberately withdrawn — and the
// only symptom would be a shop that quietly disagrees with the console.
//
// So this is a bootstrap, not a sync. Use SeedOverwrite when you genuinely
// intend to reset products to the values in code.
//
// Trust tiles are still replaced: nothing else can edit them, so the seed is
// the only way they can ever change.
func (r *Repository) Seed(ctx context.Context, products []Product, tiles []TrustTile) error {
	return r.seed(ctx, products, tiles, false)
}

// SeedOverwrite resets every editable field of existing products to the values
// in code, discarding administrator edits.
//
// Stock and images survive even here: stock is a live count that the seed has
// no business rewinding, and images were uploaded through the console and
// exist nowhere in code.
func (r *Repository) SeedOverwrite(ctx context.Context, products []Product, tiles []TrustTile) error {
	return r.seed(ctx, products, tiles, true)
}

func (r *Repository) seed(ctx context.Context, products []Product, tiles []TrustTile, overwrite bool) error {
	for _, p := range products {
		if err := p.Validate(); err != nil {
			return fmt.Errorf("catalogue: seed: %w", err)
		}
	}

	for _, p := range products {
		// Every editable field. Which operator these go under is the whole
		// difference between the two entry points above.
		fields := bson.D{
			{Key: fieldFamily, Value: string(p.Family)},
			{Key: fieldForm, Value: string(p.Form)},
			{Key: "name", Value: p.Name},
			{Key: "stat", Value: p.Stat},
			{Key: "stat2", Value: p.Stat2},
			{Key: "blurb", Value: p.Blurb},
			{Key: "grad", Value: p.Grad},
			{Key: fieldPosition, Value: p.Position},
			{Key: "mrp", Value: p.MRP},
			{Key: "price", Value: p.Price},
			{Key: fieldActive, Value: p.Active},
			{Key: "rating", Value: p.Rating},
			{Key: "badges", Value: p.Badges},
			{Key: "nutrition", Value: p.Nutrition},
		}

		// Never overwritten by either path: stock is a live count, and images
		// were uploaded through the console and exist nowhere in code.
		onInsert := bson.D{
			{Key: fieldStock, Value: p.Stock},
			{Key: fieldImages, Value: []Image{}},
		}

		var update bson.D
		if overwrite {
			update = bson.D{
				{Key: opSet, Value: fields},
				{Key: "$setOnInsert", Value: onInsert},
			}
		} else {
			// Everything under $setOnInsert: a product that already exists is
			// not modified at all.
			update = bson.D{{Key: "$setOnInsert", Value: append(fields, onInsert...)}}
		}

		if _, err := r.products.UpdateOne(ctx,
			bson.D{{Key: fieldID, Value: string(p.ID)}}, update,
			options.UpdateOne().SetUpsert(true),
		); err != nil {
			return fmt.Errorf("catalogue: seed product %q: %w", p.ID, err)
		}
	}

	trust := bson.D{{Key: opSet, Value: bson.D{{Key: "tiles", Value: tiles}}}}
	if _, err := r.content.UpdateOne(ctx,
		bson.D{{Key: fieldID, Value: trustContentID}}, trust,
		options.UpdateOne().SetUpsert(true),
	); err != nil {
		return fmt.Errorf("catalogue: seed trust tiles: %w", err)
	}
	return nil
}

// ListAll returns every product, including retired ones, ordered by position.
// Used by the admin catalogue console, where retired products must be
// accessible so they can be brought back with a PUT.
func (r *Repository) ListAll(ctx context.Context) ([]Product, error) {
	return r.find(ctx, bson.D{}, "list all products")
}

// Create inserts a new product document. Returns ErrDuplicateProduct when a
// product with the same _id already exists — _id is the join key of every
// cart line and order line, so silent reuse is never acceptable.
func (r *Repository) Create(ctx context.Context, p Product) error {
	if _, err := r.products.InsertOne(ctx, p); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ErrDuplicateProduct
		}
		return fmt.Errorf("catalogue: create product: %w", err)
	}
	return nil
}

// Update replaces every editable field of an existing product. Returns
// ErrProductNotFound when no document matches the id. _id is never sent in
// $set — rewriting the join key would orphan every cart line and order line
// that references it.
func (r *Repository) Update(ctx context.Context, p Product) error {
	res, err := r.products.UpdateOne(ctx,
		bson.D{{Key: fieldID, Value: string(p.ID)}},
		bson.D{{Key: opSet, Value: bson.D{
			{Key: fieldFamily, Value: string(p.Family)},
			{Key: fieldForm, Value: string(p.Form)},
			{Key: "name", Value: p.Name},
			{Key: "stat", Value: p.Stat},
			{Key: "stat2", Value: p.Stat2},
			{Key: "blurb", Value: p.Blurb},
			{Key: "grad", Value: p.Grad},
			{Key: fieldPosition, Value: p.Position},
			{Key: fieldImages, Value: p.Images},
			{Key: "mrp", Value: p.MRP},
			{Key: "price", Value: p.Price},
			{Key: fieldStock, Value: p.Stock},
			{Key: fieldActive, Value: p.Active},
			{Key: "rating", Value: p.Rating},
			{Key: "badges", Value: p.Badges},
			{Key: "nutrition", Value: p.Nutrition},
		}}},
	)
	if err != nil {
		return fmt.Errorf("catalogue: update product: %w", err)
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("%w: %s", ErrProductNotFound, p.ID)
	}
	return nil
}

// Retire sets active:false on the product with the given id via $set, leaving
// the document intact so past orders can still reference it. Returns
// ErrProductNotFound only when no document matches at all — an already-retired
// product is a no-op, not an error.
func (r *Repository) Retire(ctx context.Context, id ID) error {
	res, err := r.products.UpdateOne(ctx,
		bson.D{{Key: fieldID, Value: string(id)}},
		bson.D{{Key: opSet, Value: bson.D{{Key: fieldActive, Value: false}}}},
	)
	if err != nil {
		return fmt.Errorf("catalogue: retire product: %w", err)
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("%w: %s", ErrProductNotFound, id)
	}
	return nil
}

// EnsureIndexes creates the indexes schema.md specifies for this package.
func (r *Repository) EnsureIndexes(ctx context.Context) error {
	_, err := r.products.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: fieldForm, Value: 1}, {Key: fieldPosition, Value: 1}},
			Options: options.Index().SetName("form_1_position_1"),
		},
		{
			Keys:    bson.D{{Key: fieldFamily, Value: 1}, {Key: fieldPosition, Value: 1}},
			Options: options.Index().SetName("family_1_position_1"),
		},
	})
	if err != nil {
		return fmt.Errorf("catalogue: ensure products indexes: %w", err)
	}
	return nil
}
