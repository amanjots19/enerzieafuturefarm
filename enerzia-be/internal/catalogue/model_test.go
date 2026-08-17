package catalogue_test

import (
	"strings"
	"testing"

	"github.com/enerzia/enerzia-be/internal/catalogue"
)

// tabletsProduct mirrors a real catalogue entry: one sellable size.
func tabletsProduct() catalogue.Product {
	return catalogue.Product{
		ID:     "tablets-120",
		Family: "tablets",
		Form:   catalogue.FormTablets,
		Name:   "Spirulina Tablets 500 mg — 120 tabs",
		Stat:   "No binders, no fillers",
		Stat2:  "30 days",
		Blurb:  "Pure spirulina pressed into 500 mg tablets.",
		Grad:   "radial-gradient(...)",
		MRP:    47000, Price: 38000, Stock: 25, Position: 11, Active: true,
	}
}

func TestFormValid(t *testing.T) {
	tests := []struct {
		form catalogue.Form
		want bool
	}{
		{catalogue.FormPowder, true},
		{catalogue.FormTablets, true},
		{catalogue.FormBundle, true},
		{catalogue.Form("powder"), false}, // case matters
		{catalogue.Form("Capsules"), false},
		{catalogue.Form(""), false},
	}
	for _, tt := range tests {
		t.Run(string(tt.form), func(t *testing.T) {
			if got := tt.form.Valid(); got != tt.want {
				t.Errorf("Form(%q).Valid() = %v, want %v", tt.form, got, tt.want)
			}
		})
	}
}

func TestFormsListsEveryValidForm(t *testing.T) {
	forms := catalogue.Forms()
	if len(forms) != 3 {
		t.Fatalf("Forms() returned %d entries, want 3", len(forms))
	}
	for _, f := range forms {
		if !f.Valid() {
			t.Errorf("Forms() includes %q, which Valid() rejects", f)
		}
	}
}

func TestParseFilter(t *testing.T) {
	tests := []struct {
		name         string
		raw          string
		wantForm     catalogue.Form
		wantFiltered bool
		wantOK       bool
	}{
		{name: "empty means no filter", raw: "", wantOK: true},
		{name: "all means no filter", raw: "all", wantOK: true},
		{name: "whitespace only", raw: "   ", wantOK: true},
		{name: "powder", raw: "Powder", wantForm: catalogue.FormPowder, wantFiltered: true, wantOK: true},
		{name: "tablets", raw: "Tablets", wantForm: catalogue.FormTablets, wantFiltered: true, wantOK: true},
		{name: "bundle", raw: "Bundle", wantForm: catalogue.FormBundle, wantFiltered: true, wantOK: true},
		{name: "trims space", raw: "  Bundle  ", wantForm: catalogue.FormBundle, wantFiltered: true, wantOK: true},
		// A typo must be rejected, not silently treated as "all" — otherwise
		// the shopper sees an unexplained empty grid.
		{name: "unknown value is rejected", raw: "Bundles"},
		{name: "wrong case is rejected", raw: "powder"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form, filtered, ok := catalogue.ParseFilter(tt.raw)
			if ok != tt.wantOK {
				t.Fatalf("ParseFilter(%q) ok = %v, want %v", tt.raw, ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if filtered != tt.wantFiltered || form != tt.wantForm {
				t.Errorf("ParseFilter(%q) = (%q, %v), want (%q, %v)",
					tt.raw, form, filtered, tt.wantForm, tt.wantFiltered)
			}
		})
	}
}

func TestDiscountPercentMatchesTheFrontend(t *testing.T) {
	// Every real catalogue price, with the percentage the shipped UI shows.
	tests := []struct {
		name  string
		mrp   int64
		price int64
		want  int
	}{
		{"powder 100g", 25000, 20000, 20},
		{"powder 250g", 56000, 45000, 20},
		{"tablets 60", 25000, 20000, 20},
		{"tablets 120", 47000, 38000, 19},
		{"tablets 300", 110000, 85000, 23},
		{"refill 250g", 52000, 42000, 19},
		{"refill 500g", 98000, 76000, 22},
		{"bundle starter", 50000, 38000, 24},
		{"bundle family", 103000, 79000, 23},
		{"no discount", 20000, 20000, 0},
		{"zero mrp does not divide by zero", 0, 20000, 0},
		{"negative mrp is treated as no discount", -100, 20000, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := catalogue.Product{MRP: tt.mrp, Price: tt.price}
			if got := p.DiscountPercent(); got != tt.want {
				t.Errorf("DiscountPercent() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSavings(t *testing.T) {
	p := catalogue.Product{MRP: 47000, Price: 38000}
	if got := p.Savings(); got != 9000 {
		t.Errorf("Savings() = %d, want 9000 paise", got)
	}
}

func TestSoldOutIsDerivedFromStock(t *testing.T) {
	tests := []struct {
		stock int
		want  bool
	}{
		{10, false},
		{1, false},
		{0, true},
		{-1, true}, // defensive: should never happen, but is not "in stock"
	}
	for _, tt := range tests {
		p := catalogue.Product{Stock: tt.stock}
		if got := p.SoldOut(); got != tt.want {
			t.Errorf("stock %d: SoldOut() = %v, want %v", tt.stock, got, tt.want)
		}
	}
}

func TestCanFulfil(t *testing.T) {
	p := catalogue.Product{Stock: 3}

	tests := []struct {
		qty  int
		want bool
	}{
		{1, true}, {3, true}, {4, false}, {0, false}, {-1, false},
	}
	for _, tt := range tests {
		if got := p.CanFulfil(tt.qty); got != tt.want {
			t.Errorf("CanFulfil(%d) with stock 3 = %v, want %v", tt.qty, got, tt.want)
		}
	}
}

func TestAsSibling(t *testing.T) {
	p := tabletsProduct()
	p.Stock = 0

	s := p.AsSibling()
	if s.ID != p.ID || s.Name != p.Name || s.Price != p.Price || s.MRP != p.MRP {
		t.Errorf("AsSibling() = %+v, does not carry the product's identity", s)
	}
	if !s.SoldOut {
		t.Error("a sold-out product must cross-link as sold out")
	}
}

func TestValidateAcceptsARealProduct(t *testing.T) {
	if err := tabletsProduct().Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestValidateImages(t *testing.T) {
	const validURL = "https://res.cloudinary.com/enerzia/image/upload/v1/products/abc.jpg"

	makeImages := func(n int) []catalogue.Image {
		imgs := make([]catalogue.Image, n)
		for i := range imgs {
			imgs[i] = catalogue.Image{URL: validURL}
		}
		return imgs
	}

	tests := []struct {
		name    string
		images  []catalogue.Image
		wantErr string
	}{
		{"nil is valid", nil, ""},
		{"empty slice is valid", []catalogue.Image{}, ""},
		{"exactly MaxImages is valid", makeImages(catalogue.MaxImages), ""},
		{"MaxImages+1 is rejected", makeImages(catalogue.MaxImages + 1), "at most 5 images"},
		{"blank url is rejected", []catalogue.Image{{URL: ""}}, "blank url"},
		{"whitespace url is rejected", []catalogue.Image{{URL: "   "}}, "blank url"},
		{"http url is rejected", []catalogue.Image{{URL: "http://example.com/img.jpg"}}, "https://"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tabletsProduct()
			p.Images = tt.images
			err := p.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("Validate() error = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() error = nil, want one containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Validate() error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateRejectsBadProducts(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*catalogue.Product)
		wantMsg string
	}{
		{"missing id", func(p *catalogue.Product) { p.ID = "" }, "product id is required"},
		{"blank id", func(p *catalogue.Product) { p.ID = "   " }, "product id is required"},
		{"missing name", func(p *catalogue.Product) { p.Name = "  " }, "name is required"},
		{"unknown form", func(p *catalogue.Product) { p.Form = "Capsules" }, `unknown form "Capsules"`},
		{"missing family", func(p *catalogue.Product) { p.Family = "" }, "family is required"},
		{"zero mrp", func(p *catalogue.Product) { p.MRP = 0 }, "mrp must be positive"},
		{"negative price", func(p *catalogue.Product) { p.Price = -1 }, "price must be positive"},
		// A price above MRP would show a negative discount and overcharge.
		{"price above mrp", func(p *catalogue.Product) { p.Price = 99000 }, "exceeds mrp"},
		{"negative stock", func(p *catalogue.Product) { p.Stock = -1 }, "stock is negative"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tabletsProduct()
			tt.mutate(&p)

			err := p.Validate()
			if err == nil {
				t.Fatalf("Validate() error = nil, want one containing %q", tt.wantMsg)
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("Validate() error = %q, want it to contain %q", err, tt.wantMsg)
			}
		})
	}
}
