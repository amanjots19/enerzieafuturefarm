package cart

// Delivery pricing, in paise. product.md §4: free over ₹499, otherwise ₹50.
//
// These two constants are the only place the delivery rule is expressed. The
// handler, the order snapshot and the storefront all read the computed result
// rather than re-deriving it, so changing the rule here changes it everywhere
// — do not copy either value into a caller.
const (
	// FreeShippingThreshold is the subtotal at or above which delivery is
	// free. ₹499.
	FreeShippingThreshold int64 = 49900
	// ShippingFee applies below the threshold. ₹50.
	ShippingFee int64 = 5000
)

// ComputeTotals sums a resolved cart.
//
// The order matters and mirrors product.md §4 exactly: MRP and subtotal are
// independent sums, savings is their difference, and shipping is decided from
// the subtotal — never from the MRP, or a heavily discounted cart would earn
// free delivery it has not paid for.
func ComputeTotals(lines []Line) Totals {
	var totals Totals
	for _, l := range lines {
		// A withdrawn variant has no price to charge; it is shown so the
		// shopper can remove it, not summed.
		if l.Unavailable {
			continue
		}
		totals.MRPTotal += l.UnitMRP * int64(l.Qty)
		totals.Subtotal += l.UnitPrice * int64(l.Qty)
	}
	totals.Savings = totals.MRPTotal - totals.Subtotal
	totals.Shipping = shippingFor(totals.Subtotal)
	totals.Total = totals.Subtotal + totals.Shipping
	return totals
}

// shippingFor returns the delivery charge for a subtotal.
//
// An empty cart is free rather than ₹50: charging delivery on nothing would
// show a total on an empty cart.
func shippingFor(subtotal int64) int64 {
	if subtotal == 0 || subtotal >= FreeShippingThreshold {
		return 0
	}
	return ShippingFee
}

// ComputeFreeShipping describes how far a subtotal is from free delivery.
//
// An empty cart is reported as not qualified with the full threshold
// remaining: it costs nothing to deliver nothing, but telling a shopper with
// an empty cart that they have "earned" free delivery would be nonsense.
func ComputeFreeShipping(subtotal int64) FreeShipping {
	if subtotal > 0 && subtotal >= FreeShippingThreshold {
		return FreeShipping{ThresholdAmount: FreeShippingThreshold, Qualified: true}
	}
	remaining := FreeShippingThreshold - subtotal
	if remaining < 0 {
		remaining = 0
	}
	return FreeShipping{ThresholdAmount: FreeShippingThreshold, RemainingAmount: remaining}
}

// ItemCount is the sum of quantities, which drives the header badge — not the
// number of distinct lines.
func ItemCount(lines []Line) int {
	var n int
	for _, l := range lines {
		if l.Unavailable {
			continue
		}
		n += l.Qty
	}
	return n
}
