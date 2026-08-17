package catalogue_test

import (
	"testing"

	"github.com/enerzia/enerzia-be/internal/catalogue"
)

// validProduct returns a fully valid Product for ValidateForWrite so each test
// can modify exactly the field under test without setting up everything else.
func validProduct() catalogue.Product {
	return catalogue.Product{
		ID:       "tablets-120",
		Name:     "Spirulina Tablets",
		Family:   "tablets",
		Form:     catalogue.FormTablets,
		MRP:      47000,
		Price:    38000,
		Stock:    100,
		Position: 0,
		Active:   true,
		Rating:   catalogue.Rating{Score: 4.8, Count: 312},
	}
}

func TestValidateForWriteAcceptsAValidProduct(t *testing.T) {
	p := validProduct()
	if _, ok := catalogue.ValidateForWrite(p); !ok {
		t.Error("ValidateForWrite() = false, want true for a valid product")
	}
}

// TestValidateForWriteTable covers every row from roadmap.md §Admin in order.
func TestValidateForWriteTable(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*catalogue.Product)
		field   string
		message string
	}{
		// Row 1 — id: blank
		{
			name:    "blank id",
			mutate:  func(p *catalogue.Product) { p.ID = "" },
			field:   "id",
			message: "Product id must be lowercase letters, numbers and hyphens.",
		},
		// Row 1 — id: whitespace only
		{
			name:    "whitespace id",
			mutate:  func(p *catalogue.Product) { p.ID = "   " },
			field:   "id",
			message: "Product id must be lowercase letters, numbers and hyphens.",
		},
		// Row 1 — id: uppercase
		{
			name:    "uppercase id",
			mutate:  func(p *catalogue.Product) { p.ID = "Tablets-120" },
			field:   "id",
			message: "Product id must be lowercase letters, numbers and hyphens.",
		},
		// Row 1 — id: leading hyphen
		{
			name:    "leading-hyphen id",
			mutate:  func(p *catalogue.Product) { p.ID = "-tablets-120" },
			field:   "id",
			message: "Product id must be lowercase letters, numbers and hyphens.",
		},
		// Row 1 — id: consecutive hyphens
		{
			name:    "consecutive-hyphens id",
			mutate:  func(p *catalogue.Product) { p.ID = "tablets--120" },
			field:   "id",
			message: "Product id must be lowercase letters, numbers and hyphens.",
		},
		// Row 2 — name
		{
			name:    "blank name",
			mutate:  func(p *catalogue.Product) { p.Name = "" },
			field:   "name",
			message: "Please enter a product name.",
		},
		{
			name:    "whitespace name",
			mutate:  func(p *catalogue.Product) { p.Name = "  " },
			field:   "name",
			message: "Please enter a product name.",
		},
		// Row 3 — family
		{
			name:    "blank family",
			mutate:  func(p *catalogue.Product) { p.Family = "" },
			field:   "family",
			message: "Please enter the family this product belongs to.",
		},
		// Row 4 — form
		{
			name:    "unknown form",
			mutate:  func(p *catalogue.Product) { p.Form = "Capsules" },
			field:   "form",
			message: "Choose a form: Powder, Tablets or Bundle.",
		},
		// Row 5 — mrp zero
		{
			name:    "zero mrp",
			mutate:  func(p *catalogue.Product) { p.MRP = 0 },
			field:   "mrp",
			message: "MRP must be more than zero.",
		},
		// Row 5 — mrp negative
		{
			name:    "negative mrp",
			mutate:  func(p *catalogue.Product) { p.MRP = -1 },
			field:   "mrp",
			message: "MRP must be more than zero.",
		},
		// Row 6 — price zero
		{
			name:    "zero price",
			mutate:  func(p *catalogue.Product) { p.Price = 0 },
			field:   "price",
			message: "Price must be more than zero and not above MRP.",
		},
		// Row 6 — price above mrp
		{
			name:    "price above mrp",
			mutate:  func(p *catalogue.Product) { p.Price = p.MRP + 1 },
			field:   "price",
			message: "Price must be more than zero and not above MRP.",
		},
		// Row 7 — stock
		{
			name:    "negative stock",
			mutate:  func(p *catalogue.Product) { p.Stock = -1 },
			field:   "stock",
			message: "Stock cannot be negative.",
		},
		// Row 8 — position
		{
			name:    "negative position",
			mutate:  func(p *catalogue.Product) { p.Position = -1 },
			field:   "position",
			message: "Position cannot be negative.",
		},
		// Row 9 — images count
		{
			name: "too many images",
			mutate: func(p *catalogue.Product) {
				p.Images = []catalogue.Image{
					{URL: "https://cdn.example.com/a.jpg", PublicID: "a"},
					{URL: "https://cdn.example.com/b.jpg", PublicID: "b"},
					{URL: "https://cdn.example.com/c.jpg", PublicID: "c"},
					{URL: "https://cdn.example.com/d.jpg", PublicID: "d"},
					{URL: "https://cdn.example.com/e.jpg", PublicID: "e"},
					{URL: "https://cdn.example.com/f.jpg", PublicID: "f"},
				}
			},
			field:   "images",
			message: "A product can have at most five images.",
		},
		// Row 9b — image url not https
		{
			name: "http image url",
			mutate: func(p *catalogue.Product) {
				p.Images = []catalogue.Image{
					{URL: "http://cdn.example.com/a.jpg", PublicID: "a"},
				}
			},
			field:   "images",
			message: "Image links must start with https://.",
		},
		// Row 9b — blank image url (no scheme)
		{
			name: "blank image url",
			mutate: func(p *catalogue.Product) {
				p.Images = []catalogue.Image{
					{URL: "", PublicID: "a"},
				}
			},
			field:   "images",
			message: "Image links must start with https://.",
		},
		// publicId blank check (additional)
		{
			name: "blank publicId",
			mutate: func(p *catalogue.Product) {
				p.Images = []catalogue.Image{
					{URL: "https://cdn.example.com/a.jpg", PublicID: ""},
				}
			},
			field:   "images",
			message: "Every image needs a Cloudinary public id.",
		},
		// Row 10 — rating.score above 5
		{
			name:    "rating score above 5",
			mutate:  func(p *catalogue.Product) { p.Rating.Score = 5.1 },
			field:   "rating.score",
			message: "Rating must be between 0 and 5.",
		},
		// Row 10 — rating.score negative
		{
			name:    "negative rating score",
			mutate:  func(p *catalogue.Product) { p.Rating.Score = -0.1 },
			field:   "rating.score",
			message: "Rating must be between 0 and 5.",
		},
		// Row 11 — rating.count
		{
			name:    "negative rating count",
			mutate:  func(p *catalogue.Product) { p.Rating.Count = -1 },
			field:   "rating.count",
			message: "Review count cannot be negative.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := validProduct()
			tc.mutate(&p)

			problem, ok := catalogue.ValidateForWrite(p)
			if ok {
				t.Fatal("ValidateForWrite() = true, want false")
			}
			if problem.Field != tc.field {
				t.Errorf("problem.Field = %q, want %q", problem.Field, tc.field)
			}
			if problem.Message != tc.message {
				t.Errorf("problem.Message = %q, want %q", problem.Message, tc.message)
			}
		})
	}
}

// TestValidateForWriteOrderMatchesRoadmap asserts that failures are returned in
// the order the roadmap table specifies. A product missing everything should
// fail on id first, not on the last field.
func TestValidateForWriteOrderMatchesRoadmap(t *testing.T) {
	// A product that fails every rule — the result must be the first one.
	p := catalogue.Product{
		ID:    "", // row 1 fails first
		Name:  "",
		MRP:   -1,
		Price: -1,
		Stock: -1,
	}
	problem, ok := catalogue.ValidateForWrite(p)
	if ok {
		t.Fatal("ValidateForWrite() = true, want false for an empty product")
	}
	if problem.Field != "id" {
		t.Errorf("first problem field = %q, want id (roadmap table row 1)", problem.Field)
	}
}

// TestValidateForWriteAcceptsValidImages tests that an image with a proper https
// url and non-blank publicId passes validation.
func TestValidateForWriteAcceptsValidImages(t *testing.T) {
	p := validProduct()
	p.Images = []catalogue.Image{
		{URL: "https://cdn.example.com/a.jpg", PublicID: "enerzia/products/abc", Alt: "product front"},
	}
	if _, ok := catalogue.ValidateForWrite(p); !ok {
		t.Error("ValidateForWrite() = false for a product with a valid image, want true")
	}
}

// TestValidateForWriteAcceptsZeroStock asserts that stock=0 is valid (sold out,
// not invalid).
func TestValidateForWriteAcceptsZeroStock(t *testing.T) {
	p := validProduct()
	p.Stock = 0
	if _, ok := catalogue.ValidateForWrite(p); !ok {
		t.Error("ValidateForWrite() = false for stock=0, want true")
	}
}

// TestValidateForWriteAcceptsPriceEqualMRP asserts that price==mrp is valid
// (no discount, but not above mrp).
func TestValidateForWriteAcceptsPriceEqualMRP(t *testing.T) {
	p := validProduct()
	p.Price = p.MRP
	if _, ok := catalogue.ValidateForWrite(p); !ok {
		t.Error("ValidateForWrite() = false for price==mrp, want true")
	}
}
