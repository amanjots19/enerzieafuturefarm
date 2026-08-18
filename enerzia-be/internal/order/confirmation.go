package order

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

	"github.com/enerzia/enerzia-be/internal/email"
)

// confirmationLine is one bought item as the email shows it.
type confirmationLine struct {
	Name  string
	Qty   int
	Total string
}

// confirmationData is the flat view the templates render.
//
// Flat rather than the Order itself, for the same reason the shipping label is:
// the template cannot then reach a field that should not be in a customer's
// inbox. A Razorpay payment id is the obvious example.
type confirmationData struct {
	OrderID  string
	Name     string
	ETA      string
	Lines    []confirmationLine
	Subtotal string
	Savings  string
	Shipping string
	Total    string
	Paid     string

	AddressName  string
	AddressLine1 string
	AddressCity  string
	AddressState string
	AddressPin   string
	Phone        string
}

// rupees renders paise with Indian digit grouping: 105000 → "₹1,050",
// 124790 → "₹1,247.90".
//
// Written out rather than pulled from a library because Indian grouping is not
// every-three-digits: it is the last three, then twos. A naive formatter turns
// ₹12,34,567 into ₹1,234,567, which reads as a different number to the person
// it is being shown to.
//
// Paise are shown when present and omitted when zero. An earlier version did
// integer division and dropped them, so an order of ₹1,247.90 told the customer
// they had paid ₹1,247. Prices are not always whole rupees — tablets-120 is
// ₹798.90 — so this is a real case, not a hypothetical.
func rupees(paise int64) string {
	neg := paise < 0
	if neg {
		paise = -paise
	}
	whole := paise / 100
	frac := paise % 100

	digits := fmt.Sprintf("%d", whole)
	var out string
	if len(digits) <= 3 {
		out = digits
	} else {
		last3 := digits[len(digits)-3:]
		rest := digits[:len(digits)-3]
		var groups []string
		for len(rest) > 2 {
			groups = append([]string{rest[len(rest)-2:]}, groups...)
			rest = rest[:len(rest)-2]
		}
		if rest != "" {
			groups = append([]string{rest}, groups...)
		}
		out = strings.Join(groups, ",") + "," + last3
	}

	if frac != 0 {
		out += fmt.Sprintf(".%02d", frac)
	}

	sign := ""
	if neg {
		sign = "-"
	}
	return sign + "₹" + out
}

var confirmationHTML = template.Must(template.New("confirmation").Parse(`<!doctype html>
<html lang="en">
<body style="margin:0;padding:0;background:#f4f6f4;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#f4f6f4;padding:24px 12px;">
<tr><td align="center">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:560px;background:#ffffff;border-radius:10px;padding:28px;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Arial,sans-serif;color:#16281f;">

  <tr><td style="font-size:20px;font-weight:700;padding-bottom:4px;">Thanks, {{.Name}} — your order is confirmed.</td></tr>
  <tr><td style="font-size:15px;color:#4a5a52;padding-bottom:20px;">
    Order <strong>{{.OrderID}}</strong>{{if .ETA}} &middot; arriving {{.ETA}}{{end}}
  </td></tr>

  <tr><td style="padding-bottom:8px;font-size:12px;letter-spacing:.08em;text-transform:uppercase;color:#7b8a83;">What you ordered</td></tr>
  <tr><td style="padding-bottom:16px;">
    <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="font-size:15px;">
      {{range .Lines}}
      <tr>
        <td style="padding:6px 0;border-bottom:1px solid #eef1ef;">{{.Qty}} &times; {{.Name}}</td>
        <td style="padding:6px 0;border-bottom:1px solid #eef1ef;text-align:right;white-space:nowrap;">{{.Total}}</td>
      </tr>
      {{end}}
      <tr><td style="padding:6px 0;color:#4a5a52;">Subtotal</td><td style="padding:6px 0;text-align:right;">{{.Subtotal}}</td></tr>
      {{if .Savings}}<tr><td style="padding:2px 0;color:#4a5a52;">You saved</td><td style="padding:2px 0;text-align:right;">&minus; {{.Savings}}</td></tr>{{end}}
      <tr><td style="padding:2px 0;color:#4a5a52;">Delivery</td><td style="padding:2px 0;text-align:right;">{{.Shipping}}</td></tr>
      <tr>
        <td style="padding:10px 0 0;font-weight:700;border-top:1px solid #16281f;">Paid</td>
        <td style="padding:10px 0 0;font-weight:700;text-align:right;border-top:1px solid #16281f;">{{.Paid}}</td>
      </tr>
    </table>
  </td></tr>

  <tr><td style="padding-bottom:8px;font-size:12px;letter-spacing:.08em;text-transform:uppercase;color:#7b8a83;">Delivering to</td></tr>
  <tr><td style="font-size:15px;line-height:1.6;padding-bottom:20px;">
    {{.AddressName}}<br>
    {{.AddressLine1}}<br>
    {{.AddressCity}}, {{.AddressState}} {{.AddressPin}}{{if .Phone}}<br>{{.Phone}}{{end}}
  </td></tr>

  <tr><td style="font-size:13px;color:#7b8a83;border-top:1px solid #eef1ef;padding-top:16px;line-height:1.6;">
    Questions about this order? Reply to this email and quote {{.OrderID}}.
  </td></tr>

</table>
</td></tr>
</table>
</body>
</html>
`))

var confirmationText = template.Must(template.New("confirmationText").Parse(
	`Thanks, {{.Name}} — your order is confirmed.

Order {{.OrderID}}{{if .ETA}} · arriving {{.ETA}}{{end}}

WHAT YOU ORDERED
{{range .Lines}}  {{.Qty}} x {{.Name}} — {{.Total}}
{{end}}
  Subtotal   {{.Subtotal}}
{{- if .Savings}}
  You saved  - {{.Savings}}
{{- end}}
  Delivery   {{.Shipping}}
  Paid       {{.Paid}}

DELIVERING TO
  {{.AddressName}}
  {{.AddressLine1}}
  {{.AddressCity}}, {{.AddressState}} {{.AddressPin}}
{{- if .Phone}}
  {{.Phone}}
{{- end}}

Questions about this order? Reply to this email and quote {{.OrderID}}.
`))

// BuildConfirmation renders the order-confirmation email for a placed order.
//
// html/template escapes every interpolated value, so a product name or a street
// line containing markup cannot break the message — the address is
// shopper-supplied text and is treated as such.
//
// Returns an error when the order carries no email address: there is nowhere to
// send it, and that is worth logging rather than silently skipping.
func BuildConfirmation(o Order) (email.Message, error) {
	to := strings.TrimSpace(o.ShippingAddress.Email)
	if to == "" {
		return email.Message{}, fmt.Errorf("order %s: no email address on the shipping address", o.OrderID)
	}

	lines := make([]confirmationLine, len(o.Lines))
	for i, l := range o.Lines {
		lines[i] = confirmationLine{Name: l.Name, Qty: l.Qty, Total: rupees(l.LineTotal)}
	}

	shipping := "Free"
	if o.Totals.Shipping > 0 {
		shipping = rupees(o.Totals.Shipping)
	}
	savings := ""
	if o.Totals.Savings > 0 {
		savings = rupees(o.Totals.Savings)
	}

	data := confirmationData{
		OrderID:  o.OrderID,
		Name:     firstName(o.ShippingAddress.Name),
		ETA:      etaText,
		Lines:    lines,
		Subtotal: rupees(o.Totals.Subtotal),
		Savings:  savings,
		Shipping: shipping,
		Total:    rupees(o.Totals.Total),
		Paid:     rupees(o.Totals.Total),

		AddressName:  o.ShippingAddress.Name,
		AddressLine1: o.ShippingAddress.Line1,
		AddressCity:  o.ShippingAddress.City,
		AddressState: o.ShippingAddress.State,
		AddressPin:   o.ShippingAddress.Pin,
		Phone:        o.CustomerPhone,
	}

	var htmlBuf, textBuf bytes.Buffer
	if err := confirmationHTML.Execute(&htmlBuf, data); err != nil {
		return email.Message{}, fmt.Errorf("order: render confirmation html: %w", err)
	}
	if err := confirmationText.Execute(&textBuf, data); err != nil {
		return email.Message{}, fmt.Errorf("order: render confirmation text: %w", err)
	}

	return email.Message{
		To: to,
		// The order id is in the subject because it is what a customer searches
		// their inbox for months later.
		Subject:  "Your Enerzeia order " + o.OrderID + " is confirmed",
		HTMLBody: htmlBuf.String(),
		TextBody: textBuf.String(),
	}, nil
}

// firstName is the greeting name. "Ananya Sharma" greets as "Ananya"; a
// single-word name is used whole. Falls back to a name-less greeting rather
// than printing an empty space.
func firstName(full string) string {
	f := strings.Fields(strings.TrimSpace(full))
	if len(f) == 0 {
		return "there"
	}
	return f[0]
}
