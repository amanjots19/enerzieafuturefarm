package catalogue

// Seed content for the catalogue. The source of truth is product.md §2 — keep
// the two in step, and change product.md first.
//
// Every size is its own product (schema.md decision 1), so this is nine
// entries rather than four with size lists.
//
// Prices are paise: the ₹250 tub is 25000. The frontend's lib/shop/data.ts
// holds the same numbers in rupees; task 7.1 converts at the boundary.

// rupees converts a whole-rupee price to the paise stored and served, so the
// tables below stay readable next to product.md.
func rupees(n int64) int64 { return n * 100 }

// initialStock is the opening inventory for every product. Arbitrary — there
// is no stock movement history yet, so seeding is the only way stock arrives.
const initialStock = 100

// standardRating is the review score every product shows today. Static
// content — there is no reviews feature (product.md §5).
func standardRating() Rating { return Rating{Score: 4.8, Count: 312} }

// standardBadges are the four reassurance tiles on the product detail screen.
// Stored per product, so two sizes can diverge with an edit rather than a
// migration.
func standardBadges() []Badge {
	return []Badge{
		{Title: "Lab tested", Subtitle: "Heavy metals & microbes"},
		{Title: "FSSAI licensed", Subtitle: "Lic. 100xxxxxxxxxxx"},
		{Title: "No binders", Subtitle: "100% spirulina"},
		{Title: "Free delivery", Subtitle: "On orders over ₹499"},
	}
}

// standardNutrition is the per-serving table shown on every product today.
func standardNutrition() Nutrition {
	return Nutrition{
		ServingSize: "5 g",
		Rows: []NutritionRow{
			{Key: "Protein", Value: "3.1 g"},
			{Key: "Iron", Value: "4.2 mg"},
			{Key: "Phycocyanin", Value: "750 mg"},
			{Key: "Beta-carotene", Value: "1.2 mg"},
			{Key: "Energy", Value: "19 kcal"},
		},
	}
}

// Gradients are per family, so siblings look related on the grid.
const (
	gradPowder  = "radial-gradient(120% 100% at 25% 20%,#cfe9d5,#5f9a76 55%,#1f5a41)"
	gradTablets = "radial-gradient(120% 100% at 75% 20%,#dff0d6,#7fae5c 55%,#33612c)"
	gradRefill  = "radial-gradient(120% 100% at 30% 75%,#c9e4dd,#4c8a75 55%,#1c4a3d)"
	gradBundle  = "radial-gradient(120% 100% at 65% 30%,#e8f2e5,#8ab98f 55%,#2b6349)"
)

const (
	blurbPowder = "Single-ingredient spirulina, harvested fresh and dried at low heat so the " +
		"pigment and protein survive. One teaspoon into a smoothie, dal or juice."
	blurbTablets = "Pure spirulina pressed into 500 mg tablets — nothing added. For travel " +
		"days, office desks and anyone who would rather not taste their greens."
	blurbRefill = "The same farm powder in a flat compostable pouch — refill your jar and " +
		"keep the glass."
	blurbBundle = "Powder for mornings at home, tablets for everything else. Our " +
		"most-repeated order."
)

// SeedProducts returns the whole catalogue: one entry per sellable item.
//
// A fresh slice on each call, so a caller cannot mutate the seed for everyone
// else. Ids are stable strings, which is what lets a re-seed refresh prices
// without orphaning cart lines that point at them.
func SeedProducts() []Product {
	products := []Product{
		{
			ID: "powder-100g", Family: "powder", Form: FormPowder,
			Name: "Pure Spirulina Powder — 100 g",
			Stat: "60% plant protein", Stat2: "20 servings",
			Blurb: blurbPowder, Grad: gradPowder, Position: 0,
			MRP: rupees(250), Price: rupees(200),
		},
		{
			ID: "powder-250g", Family: "powder", Form: FormPowder,
			Name: "Pure Spirulina Powder — 250 g",
			Stat: "60% plant protein", Stat2: "50 servings",
			Blurb: blurbPowder, Grad: gradPowder, Position: 1,
			MRP: rupees(560), Price: rupees(450),
		},
		{
			ID: "tablets-60", Family: "tablets", Form: FormTablets,
			Name: "Spirulina Tablets 500 mg — 60 tabs",
			Stat: "No binders, no fillers", Stat2: "15 days",
			Blurb: blurbTablets, Grad: gradTablets, Position: 10,
			MRP: rupees(250), Price: rupees(200),
		},
		{
			ID: "tablets-120", Family: "tablets", Form: FormTablets,
			Name: "Spirulina Tablets 500 mg — 120 tabs",
			Stat: "No binders, no fillers", Stat2: "30 days",
			Blurb: blurbTablets, Grad: gradTablets, Position: 11,
			MRP: rupees(470), Price: rupees(380),
		},
		{
			ID: "tablets-300", Family: "tablets", Form: FormTablets,
			Name: "Spirulina Tablets 500 mg — 300 tabs",
			Stat: "No binders, no fillers", Stat2: "75 days",
			Blurb: blurbTablets, Grad: gradTablets, Position: 12,
			MRP: rupees(1100), Price: rupees(850),
		},
		{
			// Also form Powder: the Powder filter returns the tubs and these.
			ID: "refill-250g", Family: "refill", Form: FormPowder,
			Name: "Spirulina Refill Pouch — 250 g",
			Stat: "Low-waste packaging", Stat2: "50 servings",
			Blurb: blurbRefill, Grad: gradRefill, Position: 20,
			MRP: rupees(520), Price: rupees(420),
		},
		{
			ID: "refill-500g", Family: "refill", Form: FormPowder,
			Name: "Spirulina Refill Pouch — 500 g",
			Stat: "Low-waste packaging", Stat2: "100 servings",
			Blurb: blurbRefill, Grad: gradRefill, Position: 21,
			MRP: rupees(980), Price: rupees(760),
		},
		{
			ID: "bundle-starter", Family: "bundle", Form: FormBundle,
			Name: "Daily Wellness Duo — Starter",
			Stat: "Powder 100 g + 60 tablets", Stat2: "Best value",
			Blurb: blurbBundle, Grad: gradBundle, Position: 30,
			MRP: rupees(500), Price: rupees(380),
		},
		{
			ID: "bundle-family", Family: "bundle", Form: FormBundle,
			Name: "Daily Wellness Duo — Family",
			Stat: "Powder 250 g + 120 tablets", Stat2: "Best value",
			Blurb: blurbBundle, Grad: gradBundle, Position: 31,
			MRP: rupees(1030), Price: rupees(790),
		},
	}

	for i := range products {
		products[i].Stock = initialStock
		products[i].Active = true
		products[i].Rating = standardRating()
		products[i].Badges = standardBadges()
		products[i].Nutrition = standardNutrition()
	}
	return products
}

// SeedTrustTiles returns the four statistics under the shop grid.
func SeedTrustTiles() []TrustTile {
	return []TrustTile{
		{Big: "60%+", Body: "Complete plant protein by weight, with all nine essential amino acids."},
		{Big: "FSSAI", Body: "Licensed facility; every batch third-party tested for heavy metals."},
		{Big: "0", Body: "Binders, fillers, colours or preservatives — one ingredient only."},
		{Big: "48 hrs", Body: "From harvest to sealed pack, dried below 60 °C to protect phycocyanin."},
	}
}
