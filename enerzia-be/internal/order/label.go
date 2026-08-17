package order

import (
	"bytes"
	"fmt"
	"html/template"

	"github.com/enerzia/enerzia-be/internal/config"
)

// ErrOriginNotConfigured is returned by RenderLabel when SHIP_FROM_* is unset.
// The caller answers 503: a label with blanks where the return address belongs
// is worse than no label, because a parcel that can be neither delivered nor
// returned is simply gone.
var errOriginNotConfigured = fmt.Errorf("order: shipping origin is not configured")

// labelLine is one row of the contents block on a label.
type labelLine struct {
	Qty  int
	Name string
}

// labelData is everything the template renders. It is a flat view built here
// rather than the Order itself, so the template cannot reach for a field that
// should not be printed — a payment id on a parcel helps nobody.
type labelData struct {
	OrderID string
	Date    string
	To      labelParty
	From    labelParty
	Lines   []labelLine
}

type labelParty struct {
	Name  string
	Line1 string
	City  string
	State string
	Pin   string
	Phone string
}

// labelTemplate is a 4×6 thermal shipping label.
//
// Monochrome, with rules and weight rather than filled panels: a thermal head
// prints by burning, and large solid areas smear and shorten its life. The one
// exception is the prepaid strip, which is a thin inverted bar — small enough
// not to matter and important enough to be unmissable.
//
// @page pins the physical size. Everything is in millimetres so the layout does
// not shift with the browser's font settings, and overflow is hidden so a long
// address can never push content onto a second label.
//
// The recipient block is deliberately the largest thing here: it is what a
// courier reads at arm's length off a moving van. The sizes were set against
// the worst realistic case — a 32-character name, a two-line street and six
// order lines — which fills about 80% of the label, leaving margin before the
// hidden overflow would start clipping an address.
var labelTemplate = template.Must(template.New("label").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>{{.OrderID}} — shipping label</title>
<style>
  @page { size: 4in 6in; margin: 0; }
  * { box-sizing: border-box; }
  html, body { margin: 0; padding: 0; background: #fff; color: #000; }
  body {
    width: 4in; height: 6in; overflow: hidden;
    padding: 4mm;
    font-family: "Helvetica Neue", Arial, sans-serif;
    font-size: 10pt; line-height: 1.3;
    -webkit-print-color-adjust: exact; print-color-adjust: exact;
  }
  .rule { border-top: 1.5pt solid #000; margin: 2.5mm 0; }
  .tag { font-size: 7pt; letter-spacing: .12em; text-transform: uppercase; }
  .to-name { font-size: 19pt; font-weight: 700; line-height: 1.15; }
  .to-addr { font-size: 14.5pt; line-height: 1.35; margin-top: 1mm; }
  .phone { font-size: 13pt; font-weight: 700; margin-top: 1.5mm; }
  .from { font-size: 8.5pt; line-height: 1.35; }
  .meta { display: flex; justify-content: space-between; font-size: 9pt; font-weight: 700; }
  .items { font-size: 9pt; line-height: 1.5; }
  .items div { display: flex; gap: 2mm; }
  .qty { font-weight: 700; min-width: 7mm; }
  .prepaid {
    margin-top: 2.5mm; padding: 1.5mm 0; text-align: center;
    background: #000; color: #fff;
    font-size: 10pt; font-weight: 700; letter-spacing: .06em;
  }
  @media screen {
    body { border: 1px solid #bbb; margin: 12px auto; }
  }
</style>
</head>
<body>
  <div class="tag">Deliver to</div>
  <div class="to-name">{{.To.Name}}</div>
  <div class="to-addr">
    {{.To.Line1}}<br>
    {{.To.City}}, {{.To.State}} {{.To.Pin}}
  </div>
  {{if .To.Phone}}<div class="phone">{{.To.Phone}}</div>{{end}}

  <div class="rule"></div>

  <div class="tag">Return to</div>
  <div class="from">
    {{.From.Name}}<br>
    {{.From.Line1}}<br>
    {{.From.City}}, {{.From.State}} {{.From.Pin}} &middot; {{.From.Phone}}
  </div>

  <div class="rule"></div>

  <div class="meta"><span>{{.OrderID}}</span><span>{{.Date}}</span></div>

  <div class="items">
    {{range .Lines}}<div><span class="qty">{{.Qty}}&times;</span><span>{{.Name}}</span></div>{{end}}
  </div>

  <div class="prepaid">PREPAID &mdash; DO NOT COLLECT CASH</div>
</body>
</html>
`))

// RenderLabel produces the 4×6 shipping label for an order.
//
// html/template escapes every interpolated value, so a product name or a street
// line containing "<" cannot break the markup — the shipping address is
// shopper-supplied text and is treated as such.
//
// Returns errOriginNotConfigured when the origin is unset.
func RenderLabel(o Order, origin config.ShipFromAddress) ([]byte, error) {
	if !origin.Configured() {
		return nil, errOriginNotConfigured
	}

	lines := make([]labelLine, len(o.Lines))
	for i, l := range o.Lines {
		lines[i] = labelLine{Qty: l.Qty, Name: l.Name}
	}

	data := labelData{
		OrderID: o.OrderID,
		// Date only: the hour a parcel was packed means nothing to a courier,
		// and the extra characters cost room the address needs.
		Date: o.CreatedAt.Format("2 Jan 2006"),
		To: labelParty{
			Name:  o.ShippingAddress.Name,
			Line1: o.ShippingAddress.Line1,
			City:  o.ShippingAddress.City,
			State: o.ShippingAddress.State,
			Pin:   o.ShippingAddress.Pin,
			// The frozen delivery contact, not the address's own phone: for a
			// gift they are different people, and this is the one a courier
			// should ring.
			Phone: o.CustomerPhone,
		},
		From:  labelParty(origin),
		Lines: lines,
	}

	var buf bytes.Buffer
	if err := labelTemplate.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("order: render label: %w", err)
	}
	return buf.Bytes(), nil
}
