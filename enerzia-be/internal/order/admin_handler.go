package order

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"github.com/enerzia/enerzia-be/internal/admin"
	"github.com/enerzia/enerzia-be/internal/auth"
	"github.com/enerzia/enerzia-be/internal/config"
	"github.com/enerzia/enerzia-be/internal/httpx"
)

// statusAll is the query value that lifts the status filter entirely.
const statusAll = "all"

// Messages returned by the admin order endpoints.
const (
	msgOrderNotFound = "No such order."
	msgInternal      = "Something went wrong on our side."
)

// AdminStore is the persistence surface the admin order book needs.
// *Repository satisfies it; tests substitute a stub.
type AdminStore interface {
	ListAll(ctx context.Context, f AdminFilter) ([]Order, error)
	ByOrderIDUnscoped(ctx context.Context, orderID string) (Order, error)
	SetFulfilment(ctx context.Context, orderID string, from, to Fulfilment, now time.Time) (bool, error)
}

// AdminHandler serves the admin order endpoints (roadmap.md §Admin orders).
//
// It lives in this package rather than internal/admin because it is an order
// surface: it reads order documents and speaks the order vocabulary. It borrows
// only admin.Require to gate itself, the same way catalogue's admin handler
// does.
type AdminHandler struct {
	repo   AdminStore
	parser admin.TokenParser
	logger *slog.Logger
	env    config.Environment
	// shipFrom is the label's return address. Zero value means unconfigured,
	// and the label endpoint answers 503 rather than printing blanks.
	shipFrom config.ShipFromAddress
}

// NewAdminHandler builds the admin order HTTP handler.
//
// env decides one thing only: what omitting ?status means. See defaultStatuses.
func NewAdminHandler(
	repo AdminStore,
	parser admin.TokenParser,
	logger *slog.Logger,
	env config.Environment,
	shipFrom config.ShipFromAddress,
) *AdminHandler {
	return &AdminHandler{repo: repo, parser: parser, logger: logger, env: env, shipFrom: shipFrom}
}

// defaultStatuses is what an absent ?status means (roadmap.md §GET
// /api/v1/admin/orders).
//
// In production it is the work queue: an abandoned checkout is not an order,
// and listing attempts buries the real work. Everywhere else it is everything,
// because a developer's question is usually about the attempt that failed
// rather than the ones that worked, and needing to remember ?status=all to see
// a declined payment is friction at exactly the wrong moment.
//
// Only the DEFAULT splits. An explicit ?status=… means the same in every
// environment, so a console that names what it wants cannot behave one way in
// dev and another in production. Returning nil means "no status clause at all".
func (h *AdminHandler) defaultStatuses() []Status {
	if h.env == config.EnvProduction {
		return PaidStatuses()
	}
	return nil
}

// Register mounts the admin order routes on the /api/v1 subrouter, behind the
// admin token.
func (h *AdminHandler) Register(r *mux.Router) {
	sub := r.PathPrefix("/admin").Subrouter()
	sub.Use(admin.Require(h.parser))
	sub.HandleFunc("/orders", h.listOrders).Methods(http.MethodGet)
	sub.HandleFunc("/orders/{orderId}", h.getOrder).Methods(http.MethodGet)
	sub.HandleFunc("/orders/{orderId}/fulfilment", h.setFulfilment).Methods(http.MethodPatch)
	sub.HandleFunc("/orders/{orderId}/label", h.getLabel).Methods(http.MethodGet)
}

/* ---------------------------------------------------------------- wire DTOs */

// adminCustomerDTO is who the order belongs to. Deliberately not the whole user
// document: the order book needs to reach someone about a parcel, not browse an
// account.
type adminCustomerDTO struct {
	UserID string  `json:"userId"`
	Phone  *string `json:"phone"`
}

// adminOrderDTO is the shopper's order plus what an operator needs and a
// shopper must never see.
//
// razorpaySignature is deliberately absent. It is kept on the document for
// audit and has no operational use; a console has no reason to hold an HMAC.
type adminOrderDTO struct {
	OrderID         string           `json:"orderId"`
	Status          string           `json:"status"`
	StatusLabel     string           `json:"statusLabel"`
	Fulfilment      *string          `json:"fulfilment"`
	FulfilmentLabel string           `json:"fulfilmentLabel"`
	CreatedAt       time.Time        `json:"createdAt"`
	PlacedAt        *time.Time       `json:"placedAt,omitempty"`
	Lines           []orderLineDTO   `json:"lines"`
	Totals          orderTotalsDTO   `json:"totals"`
	ShippingAddress auth.Address     `json:"shippingAddress"`
	Payment         *paymentDTO      `json:"payment,omitempty"`
	Customer        adminCustomerDTO `json:"customer"`

	RazorpayOrderID   string `json:"razorpayOrderId,omitempty"`
	RazorpayPaymentID string `json:"razorpayPaymentId,omitempty"`
}

type adminGetOrderResponse struct {
	Order adminOrderDTO `json:"order"`
}

// adminFulfilmentBody is the PATCH payload. A pointer distinguishes an absent
// field from an empty string, so "missing" and "blank" both report Required
// rather than one of them decoding as a silent no-op.
type adminFulfilmentBody struct {
	Fulfilment *string `json:"fulfilment"`
}

type adminListOrdersResponse struct {
	Orders []adminOrderDTO `json:"orders"`
	// NextBefore is the cursor for the next page, absent on the last one.
	NextBefore *time.Time `json:"nextBefore,omitempty"`
	// Count is how many orders this page holds, not a total — a total over an
	// unbounded collection costs a full scan on every page.
	Count int `json:"count"`
}

/* ----------------------------------------------------------------- handlers */

// listOrders serves GET /api/v1/admin/orders.
func (h *AdminHandler) listOrders(w http.ResponseWriter, r *http.Request) {
	filter, problem := parseAdminFilter(r.URL.Query(), h.defaultStatuses())
	if problem != "" {
		httpx.WriteError(w, httpx.CodeValidation, problem)
		return
	}

	orders, err := h.repo.ListAll(r.Context(), filter)
	if err != nil {
		h.logger.Error("admin list orders", slog.Any("error", err))
		httpx.WriteError(w, httpx.CodeInternal, msgInternal)
		return
	}

	dtos := make([]adminOrderDTO, len(orders))
	for i, o := range orders {
		dtos[i] = toAdminOrderDTO(o)
	}

	resp := adminListOrdersResponse{Orders: dtos, Count: len(dtos)}
	// A full page means there may be more. A short one is the last page, so no
	// cursor is offered — an operator should not be invited to page into
	// nothing.
	if len(orders) == filter.Limit {
		last := orders[len(orders)-1].CreatedAt
		resp.NextBefore = &last
	}

	httpx.WriteData(w, http.StatusOK, resp)
}

// getOrder serves GET /api/v1/admin/orders/{orderId}.
func (h *AdminHandler) getOrder(w http.ResponseWriter, r *http.Request) {
	orderID := mux.Vars(r)["orderId"]

	// A malformed id cannot name a real order, so it is answered as not-found
	// rather than as a validation error — and without a query. Unlike the
	// shopper's endpoint there is nothing to leak by distinguishing the cases:
	// there is no ownership dimension here, only existence.
	if !ValidOrderID(orderID) {
		httpx.WriteError(w, httpx.CodeNotFound, msgOrderNotFound)
		return
	}

	o, err := h.repo.ByOrderIDUnscoped(r.Context(), orderID)
	switch {
	case errors.Is(err, ErrOrderNotFound):
		httpx.WriteError(w, httpx.CodeNotFound, msgOrderNotFound)
		return
	case err != nil:
		h.logger.Error("admin get order", slog.Any("error", err))
		httpx.WriteError(w, httpx.CodeInternal, msgInternal)
		return
	}

	httpx.WriteData(w, http.StatusOK, adminGetOrderResponse{Order: toAdminOrderDTO(o)})
}

// setFulfilment serves PATCH /api/v1/admin/orders/{orderId}/fulfilment.
func (h *AdminHandler) setFulfilment(w http.ResponseWriter, r *http.Request) {
	orderID := mux.Vars(r)["orderId"]
	if !ValidOrderID(orderID) {
		httpx.WriteError(w, httpx.CodeNotFound, msgOrderNotFound)
		return
	}

	var body adminFulfilmentBody
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		httpx.WriteError(w, httpx.CodeBadRequest, "The request body could not be read.")
		return
	}
	if body.Fulfilment == nil || *body.Fulfilment == "" {
		httpx.WriteFieldErrors(w, []httpx.FieldError{{Field: "fulfilment", Message: msgRequired}})
		return
	}
	to := Fulfilment(*body.Fulfilment)
	// Unknown value is a 422; a known value in the wrong place is a 409. The
	// difference is "we do not have that state" versus "you cannot go there
	// from here", and an operator can act on the second.
	if !to.Valid() || to == FulfilmentNone {
		httpx.WriteFieldErrors(w, []httpx.FieldError{{
			Field:   "fulfilment",
			Message: "Unknown fulfilment " + strconv.Quote(*body.Fulfilment) + ".",
		}})
		return
	}

	o, err := h.repo.ByOrderIDUnscoped(r.Context(), orderID)
	switch {
	case errors.Is(err, ErrOrderNotFound):
		httpx.WriteError(w, httpx.CodeNotFound, msgOrderNotFound)
		return
	case err != nil:
		h.logger.Error("admin set fulfilment: load", slog.Any("error", err))
		httpx.WriteError(w, httpx.CodeInternal, msgInternal)
		return
	}

	// Fulfilling an order nobody paid for is how a parcel goes out for free.
	if o.Status != StatusPlaced {
		httpx.WriteError(w, httpx.CodeConflict,
			"Only a placed order can be fulfilled. This one is "+o.Status.Label()+".")
		return
	}
	if !CanFulfil(o.Fulfilment, to) {
		httpx.WriteError(w, httpx.CodeConflict, fulfilmentConflict(o.Fulfilment, to))
		return
	}

	now := time.Now().UTC()
	modified, err := h.repo.SetFulfilment(r.Context(), orderID, o.Fulfilment, to, now)
	if err != nil {
		h.logger.Error("admin set fulfilment: write", slog.Any("error", err))
		httpx.WriteError(w, httpx.CodeInternal, msgInternal)
		return
	}
	if !modified {
		// The guard rejected the write: somebody else moved this order between
		// the read above and the update. Reporting success would tell this
		// operator they advanced it when another person actually did.
		httpx.WriteError(w, httpx.CodeConflict,
			"Somebody else changed this order just now. Reload and try again.")
		return
	}

	o.Fulfilment = to
	o.UpdatedAt = now
	httpx.WriteData(w, http.StatusOK, adminGetOrderResponse{Order: toAdminOrderDTO(o)})
}

// getLabel serves GET /api/v1/admin/orders/{orderId}/label as printable HTML.
func (h *AdminHandler) getLabel(w http.ResponseWriter, r *http.Request) {
	orderID := mux.Vars(r)["orderId"]
	if !ValidOrderID(orderID) {
		httpx.WriteError(w, httpx.CodeNotFound, msgOrderNotFound)
		return
	}

	o, err := h.repo.ByOrderIDUnscoped(r.Context(), orderID)
	switch {
	case errors.Is(err, ErrOrderNotFound):
		httpx.WriteError(w, httpx.CodeNotFound, msgOrderNotFound)
		return
	case err != nil:
		h.logger.Error("admin get label", slog.Any("error", err))
		httpx.WriteError(w, httpx.CodeInternal, msgInternal)
		return
	}

	// Printing a label for an order nobody paid for is how a parcel goes out
	// for free.
	if !isPaid(o.Status) {
		httpx.WriteError(w, httpx.CodeConflict,
			"Only a paid order can have a label. This one is "+o.Status.Label()+".")
		return
	}

	html, err := RenderLabel(o, h.shipFrom)
	if errors.Is(err, errOriginNotConfigured) {
		// 503, not a blank return address. A parcel that can be neither
		// delivered nor returned is simply gone, along with what is in it.
		h.logger.Error("label requested but SHIP_FROM_* is unset", slog.String("orderId", orderID))
		httpx.WriteError(w, httpx.CodeUnavailable, "Shipping origin is not configured.")
		return
	}
	if err != nil {
		h.logger.Error("admin render label", slog.Any("error", err))
		httpx.WriteError(w, httpx.CodeInternal, msgInternal)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// A label is a snapshot of one moment; a cached copy could be printed after
	// the address behind it changed.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(html); err != nil {
		h.logger.Error("admin write label", slog.Any("error", err))
	}
}

// isPaid reports whether a status counts as a purchase, using the same list the
// order book defaults to — so "which orders can be labelled" and "which orders
// are real" can never drift apart.
func isPaid(s Status) bool {
	for _, p := range paidStatuses {
		if s == p {
			return true
		}
	}
	return false
}

// fulfilmentConflict explains a refused move in terms of what the operator can
// do instead. "No" on its own leaves them guessing which button was right.
func fulfilmentConflict(from, to Fulfilment) string {
	if from == to {
		return "This order is already " + from.Label() + "."
	}
	next, ok := NextFulfilment(from)
	if !ok {
		return "This order is already " + from.Label() + ", which is the last step."
	}
	return "This order is " + from.Label() + ". The next step is " + next.Label() +
		", not " + to.Label() + "."
}

/* ------------------------------------------------------------------ parsing */

// parseAdminFilter turns the query string into an AdminFilter, or returns the
// message to send as a 422. One message at a time, like every other validation
// surface in this API.
//
// defaultStatuses is what an absent ?status means; nil is "no status clause".
// It is passed in rather than decided here so this stays a pure function of its
// inputs, and so the environment split has exactly one home.
func parseAdminFilter(q map[string][]string, defaultStatuses []Status) (AdminFilter, string) {
	f := AdminFilter{Limit: AdminListDefaultLimit}

	raw := first(q, "status")
	switch raw {
	case "":
		f.Statuses = defaultStatuses
	case statusAll:
		f.Statuses = nil // no clause at all
	default:
		for _, part := range splitCSV(raw) {
			s := Status(part)
			if !s.Valid() {
				return AdminFilter{}, "Unknown status " + strconv.Quote(part) + "."
			}
			f.Statuses = append(f.Statuses, s)
		}
		if len(f.Statuses) == 0 {
			return AdminFilter{}, "status must name at least one status."
		}
	}

	if raw := first(q, "fulfilment"); raw != "" {
		for _, part := range splitCSV(raw) {
			// "none" is spelled out in the query because the stored value is
			// the empty string, and ?fulfilment= would be indistinguishable
			// from omitting the parameter.
			fv := FulfilmentNone
			if part != "none" {
				fv = Fulfilment(part)
			}
			if !fv.Valid() {
				return AdminFilter{}, "Unknown fulfilment " + strconv.Quote(part) + "."
			}
			f.Fulfilments = append(f.Fulfilments, fv)
		}
		if len(f.Fulfilments) == 0 {
			return AdminFilter{}, "fulfilment must name at least one state."
		}
	}

	if raw := first(q, "limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > AdminListMaxLimit {
			return AdminFilter{}, "limit must be a whole number between 1 and " +
				strconv.Itoa(AdminListMaxLimit) + "."
		}
		f.Limit = n
	}

	if raw := first(q, "before"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return AdminFilter{}, "before must be an RFC3339 timestamp."
		}
		f.Before = &t
	}

	return f, ""
}

// first returns the first value for key, or "".
func first(q map[string][]string, key string) string {
	if v, ok := q[key]; ok && len(v) > 0 {
		return strings.TrimSpace(v[0])
	}
	return ""
}

// splitCSV splits a comma-separated parameter and drops blank entries, so
// "placed,,packed" and a stray trailing comma are read as intended rather than
// rejected for a typo that changes nothing.
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

/* -------------------------------------------------------------- DTO mapping */

func toAdminOrderDTO(o Order) adminOrderDTO {
	dto := adminOrderDTO{
		OrderID:         o.OrderID,
		Status:          string(o.Status),
		StatusLabel:     o.Status.Label(),
		Fulfilment:      optStr(string(o.Fulfilment)),
		FulfilmentLabel: o.Fulfilment.Label(),
		CreatedAt:       o.CreatedAt,
		PlacedAt:        o.PlacedAt,
		Lines:           toOrderLineDTOs(o.Lines),
		Totals:          toTotalsDTO(o.Totals),
		ShippingAddress: o.ShippingAddress,
		Customer: adminCustomerDTO{
			UserID: o.UserID.Hex(),
			Phone:  optStr(o.CustomerPhone),
		},
		RazorpayOrderID:   o.Payment.RazorpayOrderID,
		RazorpayPaymentID: o.Payment.RazorpayPaymentID,
	}
	// Present exactly when the shopper's own endpoint would show it, so the two
	// views of one order agree. Gating on Method instead would make the block
	// vanish for a placed order whose method never got stamped — hiding a data
	// problem from the one person who could act on it, rather than showing them
	// a null.
	if o.Status == StatusPlaced {
		dto.Payment = &paymentDTO{
			Method:  optStr(string(o.Payment.Method)),
			Label:   optStr(o.Payment.Label),
			Last4:   optStr(o.Payment.Last4),
			Network: optStr(o.Payment.Network),
			VPA:     optStr(o.Payment.VPA),
		}
	}
	return dto
}
