package catalogue_test

import (
	"testing"

	"github.com/enerzia/enerzia-be/internal/catalogue"
)

// These tests pin the seed to product.md §2. If a price changes here without
// changing product.md, one of the two is wrong.

func TestSeedProductsAreAllValid(t *testing.T) {
	for _, p := range catalogue.SeedProducts() {
		t.Run(string(p.ID), func(t *testing.T) {
			if err := p.Validate(); err != nil {
				t.Errorf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestSeedIsOneEntryPerSellableItem(t *testing.T) {
	// Every size is its own product now, so the grid shows nine cards.
	got := catalogue.SeedProducts()
	if len(got) != 9 {
		t.Fatalf("SeedProducts() returned %d products, want 9", len(got))
	}
}

func TestSeedPricesAreInPaise(t *testing.T) {
	// Every real price from product.md §2, in paise.
	want := map[catalogue.ID]struct{ mrp, price int64 }{
		"powder-100g":    {25000, 20000},
		"powder-250g":    {56000, 45000},
		"tablets-60":     {25000, 20000},
		"tablets-120":    {47000, 38000},
		"tablets-300":    {110000, 85000},
		"refill-250g":    {52000, 42000},
		"refill-500g":    {98000, 76000},
		"bundle-starter": {50000, 38000},
		"bundle-family":  {103000, 79000},
	}

	seen := 0
	for _, p := range catalogue.SeedProducts() {
		w, ok := want[p.ID]
		if !ok {
			t.Errorf("unexpected product %q in the seed", p.ID)
			continue
		}
		seen++
		if p.MRP != w.mrp {
			t.Errorf("%s mrp = %d paise, want %d", p.ID, p.MRP, w.mrp)
		}
		if p.Price != w.price {
			t.Errorf("%s price = %d paise, want %d", p.ID, p.Price, w.price)
		}
	}
	if seen != len(want) {
		t.Errorf("matched %d products, want all %d", seen, len(want))
	}
}

func TestSeedIDsAreUniqueAndReadable(t *testing.T) {
	// Ids appear in URLs and in cart lines; a duplicate would merge two
	// different items into one cart row.
	seen := map[catalogue.ID]bool{}
	for _, p := range catalogue.SeedProducts() {
		if seen[p.ID] {
			t.Errorf("duplicate product id %q", p.ID)
		}
		seen[p.ID] = true
		if p.ID == "" {
			t.Error("a product has no id")
		}
	}
}

func TestSeedGroupsSiblingsByFamily(t *testing.T) {
	want := map[catalogue.Family]int{
		"powder": 2, "tablets": 3, "refill": 2, "bundle": 2,
	}

	got := map[catalogue.Family]int{}
	for _, p := range catalogue.SeedProducts() {
		got[p.Family]++
	}
	for family, n := range want {
		if got[family] != n {
			t.Errorf("family %q has %d products, want %d", family, got[family], n)
		}
	}
}

func TestSeedGivesEveryProductOpeningStock(t *testing.T) {
	// Without stock the whole catalogue would seed as sold out.
	for _, p := range catalogue.SeedProducts() {
		if p.Stock <= 0 || p.SoldOut() {
			t.Errorf("%s seeded with stock %d", p.ID, p.Stock)
		}
		if !p.Active {
			t.Errorf("%s seeded inactive", p.ID)
		}
	}
}

func TestSeedPowderFilterReturnsFourProducts(t *testing.T) {
	// product.md §2: the refill pouches are also form Powder, so the filter
	// returns two tubs plus two pouches.
	count := 0
	for _, p := range catalogue.SeedProducts() {
		if p.Form == catalogue.FormPowder {
			count++
		}
	}
	if count != 4 {
		t.Errorf("%d products have form Powder, want 4", count)
	}
}

func TestSeedPositionsAreUniqueAndKeepSiblingsTogether(t *testing.T) {
	// Position drives the grid order; siblings must not be interleaved.
	seen := map[int]bool{}
	lastOfFamily := map[catalogue.Family]int{}

	for _, p := range catalogue.SeedProducts() {
		if seen[p.Position] {
			t.Errorf("%s reuses position %d", p.ID, p.Position)
		}
		seen[p.Position] = true

		if prev, ok := lastOfFamily[p.Family]; ok && p.Position != prev+1 {
			t.Errorf("%s breaks its family's run: position %d after %d", p.ID, p.Position, prev)
		}
		lastOfFamily[p.Family] = p.Position
	}
}

func TestSeedDetailContentIsPresentOnEveryProduct(t *testing.T) {
	for _, p := range catalogue.SeedProducts() {
		t.Run(string(p.ID), func(t *testing.T) {
			if p.Rating.Score != 4.8 || p.Rating.Count != 312 {
				t.Errorf("rating = %+v, want {4.8 312}", p.Rating)
			}
			if len(p.Badges) != 4 {
				t.Errorf("%d badges, want 4", len(p.Badges))
			}
			if p.Nutrition.ServingSize != "5 g" || len(p.Nutrition.Rows) != 5 {
				t.Errorf("nutrition = %+v, want 5 g with 5 rows", p.Nutrition)
			}
			if p.Blurb == "" {
				t.Error("blurb is empty")
			}
		})
	}
}

func TestSeedProductsReturnsAnIndependentSlice(t *testing.T) {
	// A caller mutating the seed must not affect the next caller.
	first := catalogue.SeedProducts()
	first[0].Name = "mutated"
	first[0].Price = 1
	first[0].Badges[0].Title = "mutated"

	second := catalogue.SeedProducts()
	if second[0].Name == "mutated" || second[0].Price == 1 {
		t.Error("SeedProducts() shares product values between calls")
	}
	if second[0].Badges[0].Title == "mutated" {
		t.Error("SeedProducts() shares the badges backing array between calls")
	}
}

func TestSeedTrustTiles(t *testing.T) {
	tiles := catalogue.SeedTrustTiles()
	if len(tiles) != 4 {
		t.Fatalf("%d trust tiles, want 4", len(tiles))
	}

	wantBig := []string{"62%+", "FSSAI", "0", "48 hrs"}
	for i, w := range wantBig {
		if tiles[i].Big != w {
			t.Errorf("tile %d big = %q, want %q", i, tiles[i].Big, w)
		}
		if tiles[i].Body == "" {
			t.Errorf("tile %d (%s) has no body", i, tiles[i].Big)
		}
	}
}

func TestSeedTrustTilesReturnsAnIndependentSlice(t *testing.T) {
	first := catalogue.SeedTrustTiles()
	first[0].Big = "mutated"

	if catalogue.SeedTrustTiles()[0].Big == "mutated" {
		t.Error("SeedTrustTiles() shares its backing array between calls")
	}
}
