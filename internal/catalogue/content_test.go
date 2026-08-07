package catalogue_test

import (
	"strings"
	"testing"

	"github.com/enerzia/enerzia-be/internal/catalogue"
)

// detailProduct is a valid product carrying the standard detail content, so
// each case below can break exactly one thing.
func detailProduct() catalogue.Product {
	p := tabletsProduct()
	p.Rating = catalogue.Rating{Score: 4.8, Count: 312}
	p.Badges = []catalogue.Badge{{Title: "Lab tested", Subtitle: "Heavy metals & microbes"}}
	p.Nutrition = catalogue.Nutrition{
		ServingSize: "5 g",
		Rows:        []catalogue.NutritionRow{{Key: "Protein", Value: "3.1 g"}},
	}
	return p
}

func TestValidateAcceptsDetailContent(t *testing.T) {
	if err := detailProduct().Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestValidateRejectsBadDetailContent(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*catalogue.Product)
		wantMsg string
	}{
		{
			name:    "rating above the scale",
			mutate:  func(p *catalogue.Product) { p.Rating.Score = 5.1 },
			wantMsg: "outside 0..5",
		},
		{
			name:    "negative rating score",
			mutate:  func(p *catalogue.Product) { p.Rating.Score = -0.1 },
			wantMsg: "outside 0..5",
		},
		{
			name:    "negative review count",
			mutate:  func(p *catalogue.Product) { p.Rating.Count = -1 },
			wantMsg: "rating count -1 is negative",
		},
		{
			name: "badge without a title",
			mutate: func(p *catalogue.Product) {
				p.Badges = append(p.Badges, catalogue.Badge{Subtitle: "orphaned"})
			},
			wantMsg: "badge title is required",
		},
		{
			name: "nutrition row without a key",
			mutate: func(p *catalogue.Product) {
				p.Nutrition.Rows = append(p.Nutrition.Rows, catalogue.NutritionRow{Value: "1 g"})
			},
			wantMsg: "nutrition row needs both key and value",
		},
		{
			name: "nutrition row without a value",
			mutate: func(p *catalogue.Product) {
				p.Nutrition.Rows = append(p.Nutrition.Rows, catalogue.NutritionRow{Key: "Fibre"})
			},
			wantMsg: "nutrition row needs both key and value",
		},
		{
			// A table of numbers with no stated serving size is meaningless.
			name:    "rows without a serving size",
			mutate:  func(p *catalogue.Product) { p.Nutrition.ServingSize = "" },
			wantMsg: "without a serving size",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := detailProduct()
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

func TestValidateAllowsAProductWithNoDetailContent(t *testing.T) {
	// Detail content is optional at the model level; the seed supplies it.
	p := tabletsProduct()
	p.Rating = catalogue.Rating{}
	p.Badges = nil
	p.Nutrition = catalogue.Nutrition{}

	if err := p.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil for a product without detail content", err)
	}
}

func TestValidateAllowsAServingSizeWithNoRows(t *testing.T) {
	p := detailProduct()
	p.Nutrition.Rows = nil

	if err := p.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}
