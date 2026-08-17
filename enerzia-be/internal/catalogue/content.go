package catalogue

import (
	"fmt"
	"strings"
)

// maxRatingScore is the top of the review scale.
const maxRatingScore = 5.0

// Rating is the aggregate review score shown on the product detail screen.
// It is static content today — there is no reviews feature (product.md §5).
type Rating struct {
	Score float64 `bson:"score" json:"score"`
	Count int     `bson:"count" json:"count"`
}

// Badge is one reassurance tile under the buy button.
type Badge struct {
	Title    string `bson:"title"    json:"title"`
	Subtitle string `bson:"subtitle" json:"subtitle"`
}

// NutritionRow is one line of the nutrition table.
type NutritionRow struct {
	Key   string `bson:"key"   json:"key"`
	Value string `bson:"value" json:"value"`
}

// Nutrition is the per-serving breakdown shown on the product detail screen.
type Nutrition struct {
	ServingSize string         `bson:"servingSize" json:"servingSize"`
	Rows        []NutritionRow `bson:"rows"        json:"rows"`
}

// TrustTile is one of the four statistics under the shop grid.
type TrustTile struct {
	Big  string `bson:"big"  json:"big"`
	Body string `bson:"body" json:"body"`
}

// validateDetail checks the product-detail content. These fields are stored
// per product — identical across the catalogue today, but owned by the API so
// they can diverge without a frontend change (product.md §2).
func (p Product) validateDetail() error {
	if p.Rating.Score < 0 || p.Rating.Score > maxRatingScore {
		return fmt.Errorf("catalogue: product %q: rating score %v outside 0..%v",
			p.ID, p.Rating.Score, maxRatingScore)
	}
	if p.Rating.Count < 0 {
		return fmt.Errorf("catalogue: product %q: rating count %d is negative",
			p.ID, p.Rating.Count)
	}
	for _, b := range p.Badges {
		if b.Title == "" {
			return fmt.Errorf("catalogue: product %q: badge title is required", p.ID)
		}
	}
	for _, row := range p.Nutrition.Rows {
		if row.Key == "" || row.Value == "" {
			return fmt.Errorf("catalogue: product %q: nutrition row needs both key and value", p.ID)
		}
	}
	if len(p.Nutrition.Rows) > 0 && p.Nutrition.ServingSize == "" {
		return fmt.Errorf("catalogue: product %q: nutrition rows given without a serving size", p.ID)
	}
	if len(p.Images) > MaxImages {
		return fmt.Errorf("catalogue: product %q: a product can have at most %d images, got %d",
			p.ID, MaxImages, len(p.Images))
	}
	for i, img := range p.Images {
		if strings.TrimSpace(img.URL) == "" {
			return fmt.Errorf("catalogue: product %q: image %d has a blank url", p.ID, i)
		}
		if !strings.HasPrefix(img.URL, "https://") {
			return fmt.Errorf("catalogue: product %q: image %d url must be an absolute https:// url, got %q",
				p.ID, i, img.URL)
		}
	}
	return nil
}
