// Package cart owns the shopping cart: what a shopper has chosen, and what it
// costs.
//
// A stored line is a product id and a quantity — nothing more (schema.md
// decision 3). Everything monetary is resolved from the live catalogue on every
// read, so a client cannot post a price and a cart left open overnight cannot
// charge yesterday's total.
package cart

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/enerzia/enerzia-be/internal/catalogue"
)

// Quantity bounds for a single line, per roadmap.md §Cart.
const (
	// MinQty is the smallest quantity a line may hold. Zero means delete.
	MinQty = 1
	// MaxQty caps one line. Stock is checked separately and is the real limit.
	MaxQty = 99
)

// StoredLine is one row of the cart as it sits in MongoDB.
type StoredLine struct {
	ProductID catalogue.ID `bson:"productId"`
	Qty       int          `bson:"qty"`
}

// Cart is a shopper's cart document. Its _id is the owner's user id, so a
// second cart for the same shopper is impossible by construction.
type Cart struct {
	UserID    bson.ObjectID `bson:"_id"`
	Lines     []StoredLine  `bson:"lines"`
	UpdatedAt time.Time     `bson:"updatedAt"`
}

// Line is a stored line joined with live catalogue data — the shape the API
// returns.
type Line struct {
	ProductID catalogue.ID
	Name      string
	Form      catalogue.Form
	Grad      string
	Stat2     string
	UnitPrice int64
	UnitMRP   int64
	Qty       int
	LineTotal int64
	// Stock and SoldOut expose what happened to the product after it was
	// added, so the UI can explain a blocked checkout.
	Stock   int
	SoldOut bool
	// Unavailable means the product was retired from the catalogue entirely.
	Unavailable bool
}

// Blocking reports whether this line prevents checkout: sold out, withdrawn,
// or wanting more units than remain.
func (l Line) Blocking() bool { return l.Unavailable || l.SoldOut || l.Qty > l.Stock }

// Totals is the money summary. Every field is paise.
type Totals struct {
	MRPTotal int64
	Subtotal int64
	Savings  int64
	Shipping int64
	Total    int64
}

// FreeShipping describes progress toward free delivery, so the UI can render
// the "add ₹x more" nudge without doing arithmetic of its own.
type FreeShipping struct {
	ThresholdAmount int64
	Qualified       bool
	RemainingAmount int64
}

// View is a fully resolved cart.
type View struct {
	Lines        []Line
	Totals       Totals
	FreeShipping FreeShipping
	ItemCount    int
	// HasBlockingIssues is true when any line cannot be bought as it stands.
	HasBlockingIssues bool
}

// findLine returns the index of the line holding a product, or -1. Identity is
// the product id, which is why adding one already present increments.
func findLine(lines []StoredLine, productID catalogue.ID) int {
	for i, l := range lines {
		if l.ProductID == productID {
			return i
		}
	}
	return -1
}
