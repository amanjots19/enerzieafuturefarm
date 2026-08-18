package order_test

import (
	"strings"
	"testing"
	"time"

	"github.com/enerzia/enerzia-be/internal/order"
)

func TestBuildConfirmation(t *testing.T) {
	o := adminOrderFixture("EFF-483413", time.Date(2026, 8, 7, 10, 32, 11, 0, time.UTC))
	o.ShippingAddress.Email = "ananya@example.com"

	msg, err := order.BuildConfirmation(o)
	if err != nil {
		t.Fatalf("BuildConfirmation() error = %v", err)
	}

	if msg.To != "ananya@example.com" {
		t.Errorf("To = %q, want the address on the order", msg.To)
	}
	// The order id belongs in the subject: it is what a customer searches their
	// inbox for months later.
	if !strings.Contains(msg.Subject, "EFF-483413") {
		t.Errorf("Subject = %q, want it to carry the order id", msg.Subject)
	}
	if msg.HTMLBody == "" || msg.TextBody == "" {
		t.Fatal("both a plain-text and an HTML body are required")
	}

	for _, body := range []struct{ name, s string }{{"html", msg.HTMLBody}, {"text", msg.TextBody}} {
		for _, want := range []string{
			"EFF-483413",
			"Ananya",                // greeting uses the first name
			"Pure Spirulina Powder", // what they bought
			"₹250",                  // the amount paid, grouped en-IN
			"12, Anand Residency",   // where it is going
		} {
			if !strings.Contains(body.s, want) {
				t.Errorf("%s body is missing %q", body.name, want)
			}
		}
		// Nothing about the gateway belongs in a customer's inbox.
		for _, unwanted := range []string{"razorpay", "order_Pk9", "pay_Pk9", "9f2b3c"} {
			if strings.Contains(strings.ToLower(body.s), strings.ToLower(unwanted)) {
				t.Errorf("%s body leaked %q", body.name, unwanted)
			}
		}
	}
}

func TestBuildConfirmationEscapesShopperText(t *testing.T) {
	// The address is shopper-supplied and lands in an HTML email.
	o := adminOrderFixture("EFF-483413", time.Now().UTC())
	o.ShippingAddress.Email = "a@b.co"
	o.ShippingAddress.Name = `<script>alert(1)</script>`

	msg, err := order.BuildConfirmation(o)
	if err != nil {
		t.Fatalf("BuildConfirmation() error = %v", err)
	}
	if strings.Contains(msg.HTMLBody, "<script>alert(1)</script>") {
		t.Error("raw script from shopper text was rendered into the email")
	}
	if !strings.Contains(msg.HTMLBody, "&lt;script&gt;") {
		t.Error("the script tag was not escaped")
	}
}

func TestBuildConfirmationWithoutAnEmailAddress(t *testing.T) {
	// There is nowhere to send it. That is worth an error the caller logs, not
	// a silent skip.
	o := adminOrderFixture("EFF-483413", time.Now().UTC())
	o.ShippingAddress.Email = "   "

	if _, err := order.BuildConfirmation(o); err == nil {
		t.Fatal("BuildConfirmation() accepted an order with no email address")
	}
}

func TestBuildConfirmationOmitsAbsentOptionalRows(t *testing.T) {
	o := adminOrderFixture("EFF-483413", time.Now().UTC())
	o.ShippingAddress.Email = "a@b.co"
	o.CustomerPhone = "" // a pre-11.10 order
	o.Totals = order.Totals{MRPTotal: 20000, Subtotal: 20000, Savings: 0, Shipping: 0, Total: 20000}

	msg, err := order.BuildConfirmation(o)
	if err != nil {
		t.Fatalf("BuildConfirmation() error = %v", err)
	}
	// No savings means no "You saved" row — showing "You saved ₹0" reads as a
	// mistake.
	if strings.Contains(msg.TextBody, "You saved") {
		t.Error("a zero savings row was printed")
	}
	// Free delivery says Free, not ₹0.
	if !strings.Contains(msg.TextBody, "Free") {
		t.Error("free delivery was not labelled Free")
	}
}

// TestConfirmationFormatsRupeesIndianStyle pins the digit grouping. Indian
// grouping is the last three digits, then twos — a naive formatter turns
// ₹12,34,567 into ₹1,234,567, which reads as a different number to the person
// being shown it.
func TestConfirmationFormatsRupeesIndianStyle(t *testing.T) {
	tests := []struct {
		paise int64
		want  string
	}{
		{0, "₹0"},
		{20000, "₹200"},
		// Paise are real: tablets-120 is priced at 79890. An earlier version did
		// integer division and told a customer who paid ₹1,247.90 that they had
		// paid ₹1,247.
		{79890, "₹798.90"},
		{124790, "₹1,247.90"},
		{30010, "₹300.10"},
		{5, "₹0.05"},
		{12345678, "₹1,23,456.78"},
		{105000, "₹1,050"},
		{4999900, "₹49,999"},
		{12345600, "₹1,23,456"},
		{123456700, "₹12,34,567"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			o := adminOrderFixture("EFF-483413", time.Now().UTC())
			o.ShippingAddress.Email = "a@b.co"
			o.Totals.Total = tt.paise
			o.Totals.Subtotal = tt.paise
			o.Totals.MRPTotal = tt.paise
			o.Totals.Savings = 0
			o.Totals.Shipping = 0

			msg, err := order.BuildConfirmation(o)
			if err != nil {
				t.Fatalf("BuildConfirmation() error = %v", err)
			}
			if !strings.Contains(msg.TextBody, tt.want) {
				t.Errorf("body does not contain %q\n%s", tt.want, msg.TextBody)
			}
		})
	}
}

func TestBuildConfirmationGreetsWithoutAName(t *testing.T) {
	o := adminOrderFixture("EFF-483413", time.Now().UTC())
	o.ShippingAddress.Email = "a@b.co"
	o.ShippingAddress.Name = ""

	msg, err := order.BuildConfirmation(o)
	if err != nil {
		t.Fatalf("BuildConfirmation() error = %v", err)
	}
	// "Thanks, — your order" reads as a bug. A fallback greeting does not.
	if strings.Contains(msg.TextBody, "Thanks,  —") {
		t.Error("an empty name left a dangling greeting")
	}
	if !strings.Contains(msg.TextBody, "there") {
		t.Error("no fallback greeting was used")
	}
}
