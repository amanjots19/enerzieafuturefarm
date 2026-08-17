// Package catalogue holds the product domain: sellable items, stock, and
// pricing arithmetic.
//
// A product *is* a sellable item. There is no separate variant entity —
// "tablets 120" and "tablets 300" are two products, each with its own id,
// price, stock and detail page (schema.md decision 1).
//
// Everything here is pure. No I/O, no HTTP — those live in the repository and
// handler layers.
package catalogue

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

// Form is the physical shape a product ships in. Several products share a
// form, so filtering by Powder returns the tubs and the refill pouches.
type Form string

// The forms the catalogue sells.
const (
	FormPowder  Form = "Powder"
	FormTablets Form = "Tablets"
	FormBundle  Form = "Bundle"
)

// Forms lists every valid form, in the order the shop filter shows them.
func Forms() []Form { return []Form{FormPowder, FormTablets, FormBundle} }

// Valid reports whether f is a form the catalogue recognises.
func (f Form) Valid() bool {
	switch f {
	case FormPowder, FormTablets, FormBundle:
		return true
	default:
		return false
	}
}

// FilterAll is the query value meaning "do not filter". An absent filter means
// the same thing.
const FilterAll = "all"

// ParseFilter interprets the `form` query parameter. An empty value or "all"
// yields ok with a zero Form, meaning no filtering. Any unrecognised value is
// rejected rather than silently ignored, so a typo surfaces as a 400 instead
// of an empty product grid.
func ParseFilter(raw string) (form Form, filtered, ok bool) {
	switch trimmed := strings.TrimSpace(raw); trimmed {
	case "", FilterAll:
		return "", false, true
	default:
		f := Form(trimmed)
		if !f.Valid() {
			return "", false, false
		}
		return f, true, true
	}
}

// MaxImages is the maximum number of photographs a product may carry.
const MaxImages = 5

// Image is one product photograph stored as a Cloudinary URL.
//
// URL and PublicID are both required: publicId is the only handle by which an
// asset can later be deleted or transformed (schema.md §products). Alt is
// optional — most product photographs are self-explanatory.
type Image struct {
	URL      string `bson:"url"           json:"url"`
	PublicID string `bson:"publicId"      json:"publicId"`
	Alt      string `bson:"alt,omitempty" json:"alt,omitempty"`
}

// ID identifies a product, e.g. "tablets-120". Human-readable because it
// appears in URLs and in cart lines, and because there are nine of them.
type ID string

// Family groups sibling products — the three tablet sizes all carry
// "tablets". Display only: nothing about buying depends on it.
type Family string

// Product is one sellable item: an id, a price, a stock count and its own
// detail content.
//
// MRP and Price are paise (int64), never rupees and never float.
type Product struct {
	ID     ID     `bson:"_id"`
	Family Family `bson:"family"`
	Form   Form   `bson:"form"`
	Name   string `bson:"name"`
	Stat   string `bson:"stat"`
	Stat2  string `bson:"stat2"`
	Blurb  string `bson:"blurb"`
	// Grad is the CSS gradient standing in for a product photograph.
	Grad string `bson:"grad"`
	// Position orders the whole grid, so siblings sit together without the
	// client sorting.
	Position int `bson:"position"`
	// Images holds up to MaxImages Cloudinary photographs in display order.
	// Index 0 is the primary shot. An empty or nil slice means no photographs
	// have been uploaded yet; grad is the fallback in that case.
	Images []Image `bson:"images"`

	MRP   int64 `bson:"mrp"`
	Price int64 `bson:"price"`
	// Stock is units on hand. Zero or less is sold out.
	Stock int `bson:"stock"`
	// Active false retires a product without deleting it, so a past order
	// referencing it can still be explained.
	Active bool `bson:"active"`

	Rating    Rating    `bson:"rating"`
	Badges    []Badge   `bson:"badges"`
	Nutrition Nutrition `bson:"nutrition"`
}

// Savings is the per-unit discount in paise.
func (p Product) Savings() int64 { return p.MRP - p.Price }

// SoldOut reports whether the product can still be bought. Derived from stock
// rather than stored, so there is no second source of truth that can disagree
// with the number beside it.
func (p Product) SoldOut() bool { return p.Stock <= 0 }

// CanFulfil reports whether qty units are available.
func (p Product) CanFulfil(qty int) bool { return qty > 0 && p.Stock >= qty }

// DiscountPercent is the headline "n% off", rounded the same way the frontend
// does: round((1 - price/mrp) * 100). A zero or negative MRP yields 0 rather
// than dividing by zero.
func (p Product) DiscountPercent() int {
	if p.MRP <= 0 {
		return 0
	}
	return int(math.Round((1 - float64(p.Price)/float64(p.MRP)) * 100))
}

// Validate checks a product is internally consistent. It guards seed data and
// anything read back from the database, where a bad price would surface as a
// wrong charge.
func (p Product) Validate() error {
	if strings.TrimSpace(string(p.ID)) == "" {
		return errors.New("catalogue: product id is required")
	}
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("catalogue: product %q: name is required", p.ID)
	}
	if !p.Form.Valid() {
		return fmt.Errorf("catalogue: product %q: unknown form %q", p.ID, p.Form)
	}
	if strings.TrimSpace(string(p.Family)) == "" {
		return fmt.Errorf("catalogue: product %q: family is required", p.ID)
	}
	if p.MRP <= 0 {
		return fmt.Errorf("catalogue: product %q: mrp must be positive, got %d", p.ID, p.MRP)
	}
	if p.Price <= 0 {
		return fmt.Errorf("catalogue: product %q: price must be positive, got %d", p.ID, p.Price)
	}
	if p.Price > p.MRP {
		return fmt.Errorf("catalogue: product %q: price %d exceeds mrp %d", p.ID, p.Price, p.MRP)
	}
	// Negative stock is a bug upstream, not a state to tolerate.
	if p.Stock < 0 {
		return fmt.Errorf("catalogue: product %q: stock is negative (%d)", p.ID, p.Stock)
	}
	return p.validateDetail()
}

// Sibling is a lighter view of a product, used to cross-link the other sizes
// in the same family from a detail page.
type Sibling struct {
	ID      ID
	Name    string
	Stat2   string
	Price   int64
	MRP     int64
	SoldOut bool
}

// AsSibling reduces a product to its cross-link form.
func (p Product) AsSibling() Sibling {
	return Sibling{
		ID:      p.ID,
		Name:    p.Name,
		Stat2:   p.Stat2,
		Price:   p.Price,
		MRP:     p.MRP,
		SoldOut: p.SoldOut(),
	}
}
