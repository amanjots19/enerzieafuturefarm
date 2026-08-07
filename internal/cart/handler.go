package cart

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/enerzia/enerzia-be/internal/auth"
	"github.com/enerzia/enerzia-be/internal/catalogue"
	"github.com/enerzia/enerzia-be/internal/httpx"
)

// maxBodyBytes caps request bodies. Cart payloads are three short fields.
const maxBodyBytes = 4 << 10

// Handler serves the cart endpoints defined in roadmap.md §Cart. Every route
// requires a signed-in shopper.
type Handler struct {
	service *Service
	parser  auth.TokenParser
	logger  *slog.Logger
}

// NewHandler builds the cart HTTP handler.
func NewHandler(service *Service, parser auth.TokenParser, logger *slog.Logger) *Handler {
	return &Handler{service: service, parser: parser, logger: logger}
}

// Register mounts the cart routes on the /api/v1 subrouter, behind auth.
func (h *Handler) Register(r *mux.Router) {
	sub := r.PathPrefix("/cart").Subrouter()
	sub.Use(auth.Require(h.parser))

	sub.HandleFunc("", h.get).Methods(http.MethodGet)
	sub.HandleFunc("", h.clear).Methods(http.MethodDelete)
	sub.HandleFunc("/items", h.addItem).Methods(http.MethodPost)
	sub.HandleFunc("/items/{productId}", h.setQty).Methods(http.MethodPatch)
	sub.HandleFunc("/items/{productId}", h.removeItem).Methods(http.MethodDelete)
}

/* ---------------------------------------------------------------- wire DTOs */

type lineDTO struct {
	ProductID   string `json:"productId"`
	Name        string `json:"name"`
	Form        string `json:"form"`
	Grad        string `json:"grad"`
	Stat2       string `json:"stat2"`
	UnitPrice   int64  `json:"unitPrice"`
	UnitMRP     int64  `json:"unitMrp"`
	Qty         int    `json:"qty"`
	LineTotal   int64  `json:"lineTotal"`
	Stock       int    `json:"stock"`
	SoldOut     bool   `json:"soldOut"`
	Unavailable bool   `json:"unavailable,omitempty"`
}

type totalsDTO struct {
	MRPTotal int64 `json:"mrpTotal"`
	Subtotal int64 `json:"subtotal"`
	Savings  int64 `json:"savings"`
	Shipping int64 `json:"shipping"`
	Total    int64 `json:"total"`
}

type freeShippingDTO struct {
	ThresholdAmount int64 `json:"thresholdAmount"`
	Qualified       bool  `json:"qualified"`
	RemainingAmount int64 `json:"remainingAmount"`
}

type cartResponse struct {
	Lines        []lineDTO       `json:"lines"`
	Totals       totalsDTO       `json:"totals"`
	FreeShipping freeShippingDTO `json:"freeShipping"`
	ItemCount    int             `json:"itemCount"`
	// HasBlockingIssues tells the client checkout will be refused.
	HasBlockingIssues bool `json:"hasBlockingIssues"`
}

type addItemBody struct {
	ProductID string `json:"productId"`
	// Qty is a pointer so an omitted field can default to 1 while an explicit
	// 0 is still rejected as invalid.
	Qty *int `json:"qty"`
}

type setQtyBody struct {
	Qty *int `json:"qty"`
}

func toCartResponse(v View) cartResponse {
	lines := make([]lineDTO, 0, len(v.Lines))
	for _, l := range v.Lines {
		lines = append(lines, lineDTO{
			ProductID:   string(l.ProductID),
			Name:        l.Name,
			Form:        string(l.Form),
			Grad:        l.Grad,
			Stat2:       l.Stat2,
			UnitPrice:   l.UnitPrice,
			UnitMRP:     l.UnitMRP,
			Qty:         l.Qty,
			LineTotal:   l.LineTotal,
			Stock:       l.Stock,
			SoldOut:     l.SoldOut,
			Unavailable: l.Unavailable,
		})
	}
	return cartResponse{
		Lines: lines,
		Totals: totalsDTO{
			MRPTotal: v.Totals.MRPTotal,
			Subtotal: v.Totals.Subtotal,
			Savings:  v.Totals.Savings,
			Shipping: v.Totals.Shipping,
			Total:    v.Totals.Total,
		},
		FreeShipping: freeShippingDTO{
			ThresholdAmount: v.FreeShipping.ThresholdAmount,
			Qualified:       v.FreeShipping.Qualified,
			RemainingAmount: v.FreeShipping.RemainingAmount,
		},
		ItemCount:         v.ItemCount,
		HasBlockingIssues: v.HasBlockingIssues,
	}
}

/* ----------------------------------------------------------------- handlers */

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}
	view, err := h.service.View(r.Context(), userID)
	h.respond(w, r, "get cart", view, err)
}

func (h *Handler) addItem(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}

	var body addItemBody
	if !decodeJSON(w, r, &body) {
		return
	}

	qty := 1
	if body.Qty != nil {
		qty = *body.Qty
	}

	view, err := h.service.AddItem(r.Context(), userID, catalogue.ID(body.ProductID), qty)
	h.respond(w, r, "add cart item", view, err)
}

func (h *Handler) setQty(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}

	var body setQtyBody
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Qty == nil {
		httpx.WriteFieldErrors(w, []httpx.FieldError{
			{Field: "qty", Message: "Quantity is required."},
		})
		return
	}

	view, err := h.service.SetQty(r.Context(), userID, catalogue.ID(mux.Vars(r)["productId"]), *body.Qty)
	h.respond(w, r, "set cart quantity", view, err)
}

func (h *Handler) removeItem(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}
	view, err := h.service.RemoveLine(r.Context(), userID, catalogue.ID(mux.Vars(r)["productId"]))
	h.respond(w, r, "remove cart item", view, err)
}

func (h *Handler) clear(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}
	view, err := h.service.Clear(r.Context(), userID)
	h.respond(w, r, "clear cart", view, err)
}

/* ------------------------------------------------------------------ helpers */

// userID pulls the signed-in shopper from the context. Unreachable behind
// auth.Require; guarded so a routing mistake cannot serve an anonymous cart.
func (h *Handler) userID(w http.ResponseWriter, r *http.Request) (bson.ObjectID, bool) {
	id, ok := auth.UserIDFrom(r.Context())
	if !ok {
		httpx.WriteError(w, httpx.CodeUnauthorized, "Please sign in to continue.")
		return bson.NilObjectID, false
	}
	return id, true
}

// respond maps a service result to the API. Every cart endpoint returns the
// whole cart on success, so the client never has to re-fetch to redraw.
func (h *Handler) respond(w http.ResponseWriter, r *http.Request, op string, view View, err error) {
	if err != nil {
		switch {
		case errors.Is(err, ErrProductNotFound):
			httpx.WriteFieldErrors(w, []httpx.FieldError{
				{Field: "productId", Message: "That product is not available."},
			})
		case errors.Is(err, ErrInsufficientStock):
			httpx.WriteFieldErrors(w, []httpx.FieldError{
				{Field: "qty", Message: "That product is sold out or has fewer left than you asked for."},
			})
		case errors.Is(err, ErrInvalidQty):
			httpx.WriteFieldErrors(w, []httpx.FieldError{
				{Field: "qty", Message: "Choose a quantity between 1 and 99."},
			})
		case errors.Is(err, ErrLineNotFound):
			httpx.WriteError(w, httpx.CodeNotFound, "That item is not in your cart.")
		default:
			h.logger.ErrorContext(r.Context(), "cart: "+op,
				slog.Any("error", err),
				slog.String("request_id", httpx.RequestIDFrom(r.Context())),
			)
			httpx.WriteError(w, httpx.CodeInternal, "Something went wrong on our side.")
		}
		return
	}
	httpx.WriteData(w, http.StatusOK, toCartResponse(view))
}

// decodeJSON reads a JSON body, answering 400 and reporting false if it
// cannot. Unknown fields are rejected so a client typo is not swallowed.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		httpx.WriteError(w, httpx.CodeBadRequest, "The request body could not be read.")
		return false
	}
	return true
}
