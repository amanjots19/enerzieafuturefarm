package cart_test

import (
	"testing"

	"github.com/enerzia/enerzia-be/internal/cart"
)

// line builds a resolved line at the given unit prices and quantity.
func line(unitPrice, unitMRP int64, qty int) cart.Line {
	return cart.Line{UnitPrice: unitPrice, UnitMRP: unitMRP, Qty: qty, LineTotal: unitPrice * int64(qty)}
}

func TestComputeTotals(t *testing.T) {
	tests := []struct {
		name  string
		lines []cart.Line
		want  cart.Totals
	}{
		{
			name:  "empty cart",
			lines: nil,
			// Nothing to deliver, so no delivery charge and no total.
			want: cart.Totals{},
		},
		{
			name: "single line below the threshold",
			// 1 × 120 tabs: ₹380 of ₹470.
			lines: []cart.Line{line(38000, 47000, 1)},
			want: cart.Totals{
				MRPTotal: 47000, Subtotal: 38000, Savings: 9000,
				Shipping: 4900, Total: 42900,
			},
		},
		{
			name: "quantity multiplies both sums",
			// 3 × Family bundle, verified against the shipped UI: ₹2,370.
			lines: []cart.Line{line(79000, 103000, 3)},
			want: cart.Totals{
				MRPTotal: 309000, Subtotal: 237000, Savings: 72000,
				Shipping: 0, Total: 237000,
			},
		},
		{
			name: "several lines add up",
			lines: []cart.Line{
				line(20000, 25000, 1), // powder 100 g
				line(38000, 47000, 1), // tablets 120
			},
			want: cart.Totals{
				MRPTotal: 72000, Subtotal: 58000, Savings: 14000,
				Shipping: 0, Total: 58000,
			},
		},
		{
			name:  "no discount means no savings",
			lines: []cart.Line{line(50000, 50000, 1)},
			want: cart.Totals{
				MRPTotal: 50000, Subtotal: 50000, Savings: 0,
				Shipping: 0, Total: 50000,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cart.ComputeTotals(tt.lines); got != tt.want {
				t.Errorf("ComputeTotals() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestShippingBoundary(t *testing.T) {
	// The ₹499 threshold is the single most bug-prone number in the domain:
	// off by one paise either way and a shopper is charged wrongly.
	tests := []struct {
		name         string
		subtotal     int64
		wantShipping int64
	}{
		{name: "empty is free", subtotal: 0, wantShipping: 0},
		{name: "one paise", subtotal: 1, wantShipping: cart.ShippingFee},
		{name: "one paise below the threshold", subtotal: 49899, wantShipping: cart.ShippingFee},
		{name: "exactly the threshold is free", subtotal: 49900, wantShipping: 0},
		{name: "one paise above", subtotal: 49901, wantShipping: 0},
		{name: "well above", subtotal: 500000, wantShipping: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cart.ComputeTotals([]cart.Line{line(tt.subtotal, tt.subtotal, 1)})
			if got.Shipping != tt.wantShipping {
				t.Errorf("subtotal %d → shipping %d, want %d", tt.subtotal, got.Shipping, tt.wantShipping)
			}
			if got.Total != tt.subtotal+tt.wantShipping {
				t.Errorf("total = %d, want %d", got.Total, tt.subtotal+tt.wantShipping)
			}
		})
	}
}

func TestShippingIsDecidedBySubtotalNotMRP(t *testing.T) {
	// A cart whose MRP clears ₹499 but whose payable subtotal does not must
	// still be charged delivery.
	lines := []cart.Line{line(40000, 60000, 1)}

	got := cart.ComputeTotals(lines)
	if got.MRPTotal < cart.FreeShippingThreshold {
		t.Fatalf("test setup: MRP %d should exceed the threshold", got.MRPTotal)
	}
	if got.Shipping != cart.ShippingFee {
		t.Errorf("shipping = %d, want %d — the discounted subtotal is below the threshold",
			got.Shipping, cart.ShippingFee)
	}
}

func TestComputeFreeShipping(t *testing.T) {
	tests := []struct {
		name          string
		subtotal      int64
		wantQualified bool
		wantRemaining int64
	}{
		{
			// An empty cart has not "earned" free delivery.
			name: "empty", subtotal: 0,
			wantQualified: false, wantRemaining: cart.FreeShippingThreshold,
		},
		{
			// The shipped UI shows "Add ₹119 more" for a ₹380 cart.
			name: "part way there", subtotal: 38000,
			wantQualified: false, wantRemaining: 11900,
		},
		{name: "one paise short", subtotal: 49899, wantQualified: false, wantRemaining: 1},
		{name: "exactly there", subtotal: 49900, wantQualified: true, wantRemaining: 0},
		{name: "past it", subtotal: 100000, wantQualified: true, wantRemaining: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cart.ComputeFreeShipping(tt.subtotal)

			if got.Qualified != tt.wantQualified {
				t.Errorf("Qualified = %v, want %v", got.Qualified, tt.wantQualified)
			}
			if got.RemainingAmount != tt.wantRemaining {
				t.Errorf("RemainingAmount = %d, want %d", got.RemainingAmount, tt.wantRemaining)
			}
			if got.ThresholdAmount != cart.FreeShippingThreshold {
				t.Errorf("ThresholdAmount = %d, want %d", got.ThresholdAmount, cart.FreeShippingThreshold)
			}
			// Remaining must never go negative, or the UI renders "add ₹-200".
			if got.RemainingAmount < 0 {
				t.Errorf("RemainingAmount = %d, must never be negative", got.RemainingAmount)
			}
		})
	}
}

func TestFreeShippingAgreesWithTotals(t *testing.T) {
	// The nudge and the charge must never disagree: a cart told it qualifies
	// and then charged delivery is a support ticket.
	for subtotal := int64(0); subtotal <= 60000; subtotal += 1373 {
		totals := cart.ComputeTotals([]cart.Line{line(subtotal, subtotal, 1)})
		hint := cart.ComputeFreeShipping(subtotal)

		if subtotal == 0 {
			continue // empty is free but deliberately not "qualified"
		}
		if hint.Qualified != (totals.Shipping == 0) {
			t.Errorf("subtotal %d: qualified=%v but shipping=%d",
				subtotal, hint.Qualified, totals.Shipping)
		}
	}
}

func TestItemCountSumsQuantities(t *testing.T) {
	tests := []struct {
		name  string
		lines []cart.Line
		want  int
	}{
		{name: "empty", lines: nil, want: 0},
		{name: "one line one item", lines: []cart.Line{line(100, 100, 1)}, want: 1},
		// The badge counts items, not rows.
		{name: "one line three items", lines: []cart.Line{line(100, 100, 3)}, want: 3},
		{
			name:  "two lines",
			lines: []cart.Line{line(100, 100, 3), line(200, 200, 2)},
			want:  5,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cart.ItemCount(tt.lines); got != tt.want {
				t.Errorf("ItemCount() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestPricingConstantsMatchProductMd(t *testing.T) {
	if cart.FreeShippingThreshold != 49900 {
		t.Errorf("FreeShippingThreshold = %d paise, want 49900 (₹499)", cart.FreeShippingThreshold)
	}
	if cart.ShippingFee != 4900 {
		t.Errorf("ShippingFee = %d paise, want 4900 (₹49)", cart.ShippingFee)
	}
	if cart.MinQty != 1 || cart.MaxQty != 99 {
		t.Errorf("quantity bounds = %d..%d, want 1..99", cart.MinQty, cart.MaxQty)
	}
}
