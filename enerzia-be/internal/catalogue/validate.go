package catalogue

import (
	"regexp"
	"strings"
)

// FieldProblem names the field that failed validation and the message to show.
// It mirrors auth.FieldProblem so handlers can produce a consistent response
// shape without importing the auth package.
type FieldProblem struct {
	Field   string
	Message string
}

// productIDPattern matches a valid product id slug: lowercase letters, digits,
// and hyphens, with no consecutive hyphens and no leading/trailing hyphens.
// Compiled once at package init.
var productIDPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidateForWrite checks a product for creation or replacement and returns the
// first field-level problem, in the order specified by roadmap.md §Admin.
//
// ValidateForWrite is deliberately separate from Product.Validate: it returns
// an ordered shopper-facing FieldProblem; Product.Validate returns a wrapped
// internal error serving a different caller (the seeder).
func ValidateForWrite(p Product) (FieldProblem, bool) {
	// 1. id
	if strings.TrimSpace(string(p.ID)) == "" || !productIDPattern.MatchString(string(p.ID)) {
		return FieldProblem{Field: "id", Message: "Product id must be lowercase letters, numbers and hyphens."}, false
	}
	// 2. name
	if strings.TrimSpace(p.Name) == "" {
		return FieldProblem{Field: "name", Message: "Please enter a product name."}, false
	}
	// 3. family
	if strings.TrimSpace(string(p.Family)) == "" {
		return FieldProblem{Field: "family", Message: "Please enter the family this product belongs to."}, false
	}
	// 4. form
	if !p.Form.Valid() {
		return FieldProblem{Field: "form", Message: "Choose a form: Powder, Tablets or Bundle."}, false
	}
	// 5. mrp
	if p.MRP <= 0 {
		return FieldProblem{Field: "mrp", Message: "MRP must be more than zero."}, false
	}
	// 6. price
	if p.Price <= 0 || p.Price > p.MRP {
		return FieldProblem{Field: "price", Message: "Price must be more than zero and not above MRP."}, false
	}
	// 7. stock
	if p.Stock < 0 {
		return FieldProblem{Field: "stock", Message: "Stock cannot be negative."}, false
	}
	// 8. position
	if p.Position < 0 {
		return FieldProblem{Field: "position", Message: "Position cannot be negative."}, false
	}
	// 9. images — count, then per-image url and publicId
	if len(p.Images) > MaxImages {
		return FieldProblem{Field: fieldImages, Message: "A product can have at most five images."}, false
	}
	for _, img := range p.Images {
		if !strings.HasPrefix(img.URL, "https://") {
			return FieldProblem{Field: fieldImages, Message: "Image links must start with https://."}, false
		}
		if strings.TrimSpace(img.PublicID) == "" {
			return FieldProblem{Field: fieldImages, Message: "Every image needs a Cloudinary public id."}, false
		}
	}
	// 10. rating.score
	if p.Rating.Score < 0 || p.Rating.Score > maxRatingScore {
		return FieldProblem{Field: "rating.score", Message: "Rating must be between 0 and 5."}, false
	}
	// 11. rating.count
	if p.Rating.Count < 0 {
		return FieldProblem{Field: "rating.count", Message: "Review count cannot be negative."}, false
	}
	return FieldProblem{}, true
}
