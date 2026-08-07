package cart

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// cartsCollection is the collection name from schema.md.
const cartsCollection = "carts"

const fieldID = "_id"

// Repository stores carts, one document per shopper keyed by their user id.
type Repository struct {
	carts *mongo.Collection
}

// NewRepository builds the cart repository over the given database.
func NewRepository(db *mongo.Database) *Repository {
	return &Repository{carts: db.Collection(cartsCollection)}
}

// Get returns a shopper's cart. A shopper who has never added anything has no
// document, which is an empty cart rather than an error — the cart endpoint
// answers 200 with no lines, never 404.
func (r *Repository) Get(ctx context.Context, userID bson.ObjectID) (Cart, error) {
	var cart Cart
	err := r.carts.FindOne(ctx, bson.D{{Key: fieldID, Value: userID}}).Decode(&cart)

	switch {
	case errors.Is(err, mongo.ErrNoDocuments):
		return Cart{UserID: userID}, nil
	case err != nil:
		return Cart{}, fmt.Errorf("cart: get: %w", err)
	}
	return cart, nil
}

// Save writes the whole line set. Upsert means a first add creates the
// document without a separate create path.
func (r *Repository) Save(ctx context.Context, cart Cart) error {
	lines := cart.Lines
	if lines == nil {
		lines = []StoredLine{}
	}

	_, err := r.carts.UpdateOne(ctx,
		bson.D{{Key: fieldID, Value: cart.UserID}},
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "lines", Value: lines},
			{Key: "updatedAt", Value: cart.UpdatedAt},
		}}},
		options.UpdateOne().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("cart: save: %w", err)
	}
	return nil
}
