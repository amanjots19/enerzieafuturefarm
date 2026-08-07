package order

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/enerzia/enerzia-be/internal/auth"
	"github.com/enerzia/enerzia-be/internal/httpx"
)

const maxBodyBytes = 4 << 10

// Handler serves the order endpoints defined in roadmap.md §Orders.
type Handler struct {
	service *Service
	parser  auth.TokenParser
	logger  *slog.Logger
}

// NewHandler builds the order HTTP handler.
func NewHandler(service *Service, parser auth.TokenParser, logger *slog.Logger) *Handler {
	return &Handler{service: service, parser: parser, logger: logger}
}

// Register mounts the order routes on the /api/v1 subrouter, behind auth.
func (h *Handler) Register(r *mux.Router) {
	sub := r.PathPrefix("/orders").Subrouter()
	sub.Use(auth.Require(h.parser))
	sub.HandleFunc("", h.listOrders).Methods(http.MethodGet)
	sub.HandleFunc("", h.openCheckout).Methods(http.MethodPost)
	sub.HandleFunc("/{orderId}", h.getOrder).Methods(http.MethodGet)
	sub.HandleFunc("/{orderId}/payment/callback", h.paymentCallback).Methods(http.MethodPost)
}

// RegisterWebhook mounts the Razorpay webhook route on the ROOT router, outside
// the /api/v1 prefix and outside auth — Razorpay has no bearer token of ours;
// the HMAC signature is the sole authentication (roadmap.md §POST /webhooks/razorpay).
func (h *Handler) RegisterWebhook(r *mux.Router) {
	r.HandleFunc("/webhooks/razorpay", h.razorpayWebhook).Methods(http.MethodPost)
}

const etaText = "in 3–5 days" // en-dash, computed at response time
const msgRequired = "Required."

/* ---------------------------------------------------------------- wire DTOs */

type openCheckoutBody struct {
	AddressID *string `json:"addressId"`
}

type paymentCallbackBody struct {
	RazorpayOrderID   string `json:"razorpayOrderId"`
	RazorpayPaymentID string `json:"razorpayPaymentId"`
	RazorpaySignature string `json:"razorpaySignature"`
}

type orderLineDTO struct {
	ProductID string `json:"productId"`
	Name      string `json:"name"`
	Form      string `json:"form"`
	Grad      string `json:"grad"`
	UnitPrice int64  `json:"unitPrice"`
	UnitMrp   int64  `json:"unitMrp"`
	Qty       int    `json:"qty"`
	LineTotal int64  `json:"lineTotal"`
}

type orderTotalsDTO struct {
	MRPTotal int64 `json:"mrpTotal"`
	Subtotal int64 `json:"subtotal"`
	Savings  int64 `json:"savings"`
	Shipping int64 `json:"shipping"`
	Total    int64 `json:"total"`
}

type orderDTO struct {
	OrderID         string         `json:"orderId"`
	Status          string         `json:"status"`
	CreatedAt       time.Time      `json:"createdAt"`
	ExpiresAt       time.Time      `json:"expiresAt"`
	Lines           []orderLineDTO `json:"lines"`
	Totals          orderTotalsDTO `json:"totals"`
	ShippingAddress auth.Address   `json:"shippingAddress"`
}

type razorpayResponseDTO struct {
	KeyID           string `json:"keyId"`
	RazorpayOrderID string `json:"razorpayOrderId"`
	Amount          int64  `json:"amount"`
	Currency        string `json:"currency"`
}

type openCheckoutResponse struct {
	Order    orderDTO            `json:"order"`
	Razorpay razorpayResponseDTO `json:"razorpay"`
}

// paymentDTO is the payment sub-object in a placed order response. Fields are
// *string so the JSON renders as null (not "") when the method is not yet known.
type paymentDTO struct {
	Method  *string `json:"method"`
	Label   *string `json:"label"`
	Last4   *string `json:"last4"`
	Network *string `json:"network"`
	VPA     *string `json:"vpa"`
}

// placedOrderDTO is the full order shape returned by the callback and by
// GET /orders/{orderId}. It includes the payment block and ETA for placed
// orders; both fields are omitted for pending/expired orders.
type placedOrderDTO struct {
	OrderID         string         `json:"orderId"`
	Status          string         `json:"status"`
	CreatedAt       time.Time      `json:"createdAt"`
	ExpiresAt       time.Time      `json:"expiresAt"`
	PlacedAt        *time.Time     `json:"placedAt,omitempty"`
	EtaText         string         `json:"etaText,omitempty"`
	Lines           []orderLineDTO `json:"lines"`
	Totals          orderTotalsDTO `json:"totals"`
	ShippingAddress auth.Address   `json:"shippingAddress"`
	Payment         *paymentDTO    `json:"payment,omitempty"`
}

type callbackResponse struct {
	Order placedOrderDTO `json:"order"`
}

type listOrdersResponse struct {
	Orders []placedOrderDTO `json:"orders"`
}

type getOrderResponse struct {
	Order placedOrderDTO `json:"order"`
}

/* ----------------------------------------------------------------- handlers */

func (h *Handler) openCheckout(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}

	// An empty body is valid — omitting addressId selects the default address.
	var req openCheckoutBody
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		httpx.WriteError(w, httpx.CodeBadRequest, "The request body could not be read.")
		return
	}

	var addrID *bson.ObjectID
	if req.AddressID != nil && *req.AddressID != "" {
		id, err := bson.ObjectIDFromHex(*req.AddressID)
		if err != nil {
			httpx.WriteFieldErrors(w, []httpx.FieldError{
				{Field: "addressId", Message: "Invalid address ID."},
			})
			return
		}
		addrID = &id
	}

	result, err := h.service.OpenCheckout(r.Context(), userID, addrID)
	if err != nil {
		h.handleCheckoutErr(w, r, err)
		return
	}

	httpx.WriteData(w, http.StatusCreated, openCheckoutResponse{
		Order: toOrderDTO(result.Order),
		Razorpay: razorpayResponseDTO{
			KeyID:           result.RazorpayKeyID,
			RazorpayOrderID: result.RazorpayOrderID,
			Amount:          result.Amount,
			Currency:        result.Currency,
		},
	})
}

func (h *Handler) paymentCallback(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}

	orderID := mux.Vars(r)["orderId"]

	var req paymentCallbackBody
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		if errors.Is(err, io.EOF) {
			httpx.WriteFieldErrors(w, []httpx.FieldError{
				{Field: "razorpayOrderId", Message: msgRequired},
				{Field: "razorpayPaymentId", Message: msgRequired},
				{Field: "razorpaySignature", Message: msgRequired},
			})
			return
		}
		httpx.WriteError(w, httpx.CodeBadRequest, "The request body could not be read.")
		return
	}

	var fieldErrs []httpx.FieldError
	if req.RazorpayOrderID == "" {
		fieldErrs = append(fieldErrs, httpx.FieldError{Field: "razorpayOrderId", Message: msgRequired})
	}
	if req.RazorpayPaymentID == "" {
		fieldErrs = append(fieldErrs, httpx.FieldError{Field: "razorpayPaymentId", Message: msgRequired})
	}
	if req.RazorpaySignature == "" {
		fieldErrs = append(fieldErrs, httpx.FieldError{Field: "razorpaySignature", Message: msgRequired})
	}
	if len(fieldErrs) > 0 {
		httpx.WriteFieldErrors(w, fieldErrs)
		return
	}

	o, err := h.service.ConfirmPayment(r.Context(), userID, orderID, req.RazorpayOrderID, req.RazorpayPaymentID, req.RazorpaySignature)
	if err != nil {
		h.handleCallbackErr(w, r, err)
		return
	}

	httpx.WriteData(w, http.StatusOK, callbackResponse{Order: toPlacedOrderDTO(o)})
}

func (h *Handler) listOrders(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}

	orders, err := h.service.ListOrders(r.Context(), userID)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "order: list orders",
			slog.Any("error", err),
			slog.String("request_id", httpx.RequestIDFrom(r.Context())),
		)
		httpx.WriteError(w, httpx.CodeInternal, "Something went wrong on our side.")
		return
	}

	dtos := make([]placedOrderDTO, len(orders))
	for i, o := range orders {
		dtos[i] = toPlacedOrderDTO(o)
	}
	httpx.WriteData(w, http.StatusOK, listOrdersResponse{Orders: dtos})
}

func (h *Handler) getOrder(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}

	orderID := mux.Vars(r)["orderId"]
	o, err := h.service.GetOrder(r.Context(), userID, orderID)
	if err != nil {
		if errors.Is(err, ErrOrderNotFound) {
			httpx.WriteError(w, httpx.CodeNotFound, "Order not found.")
			return
		}
		h.logger.ErrorContext(r.Context(), "order: get order",
			slog.Any("error", err),
			slog.String("request_id", httpx.RequestIDFrom(r.Context())),
		)
		httpx.WriteError(w, httpx.CodeInternal, "Something went wrong on our side.")
		return
	}

	httpx.WriteData(w, http.StatusOK, getOrderResponse{Order: toPlacedOrderDTO(o)})
}

func (h *Handler) razorpayWebhook(w http.ResponseWriter, r *http.Request) {
	// Read raw bytes first — the HMAC is over these exact bytes. The body must
	// not be decoded and re-encoded before verification (schema.md §Razorpay).
	rawBody, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
	if err != nil {
		httpx.WriteError(w, httpx.CodeBadRequest, "Request body could not be read.")
		return
	}

	sig := r.Header.Get("X-Razorpay-Signature")
	if sig == "" {
		httpx.WriteError(w, httpx.CodeBadRequest, "Request signature is missing.")
		return
	}

	eventID := r.Header.Get("X-Razorpay-Event-Id")
	if eventID == "" {
		// Signature present but no event id — cannot dedupe safely.
		h.logger.WarnContext(r.Context(), "order: webhook: X-Razorpay-Event-Id is missing; acknowledging without processing")
		httpx.WriteData(w, http.StatusOK, map[string]bool{"received": true})
		return
	}

	if err := h.service.HandleWebhook(r.Context(), rawBody, eventID, sig); err != nil {
		if errors.Is(err, ErrWebhookSignatureInvalid) {
			httpx.WriteError(w, httpx.CodeBadRequest, "Request signature is invalid.")
			return
		}
		// A genuine internal failure — non-2xx so Razorpay retries.
		h.logger.ErrorContext(r.Context(), "order: webhook: internal error",
			slog.Any("error", err),
			slog.String("request_id", httpx.RequestIDFrom(r.Context())),
		)
		httpx.WriteError(w, httpx.CodeInternal, "Something went wrong on our side.")
		return
	}

	httpx.WriteData(w, http.StatusOK, map[string]bool{"received": true})
}

/* ------------------------------------------------------------------ helpers */

func (h *Handler) userID(w http.ResponseWriter, r *http.Request) (bson.ObjectID, bool) {
	id, ok := auth.UserIDFrom(r.Context())
	if !ok {
		httpx.WriteError(w, httpx.CodeUnauthorized, "Please sign in to continue.")
		return bson.NilObjectID, false
	}
	return id, true
}

func (h *Handler) handleCallbackErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrOrderNotFound):
		httpx.WriteError(w, httpx.CodeNotFound, "Order not found.")
	case errors.Is(err, ErrSignatureInvalid), errors.Is(err, ErrOrderNotPending):
		httpx.WriteError(w, httpx.CodeValidation, "We could not verify that payment.")
	default:
		h.logger.ErrorContext(r.Context(), "order: payment callback",
			slog.Any("error", err),
			slog.String("request_id", httpx.RequestIDFrom(r.Context())),
		)
		httpx.WriteError(w, httpx.CodeInternal, "Something went wrong on our side.")
	}
}

func (h *Handler) handleCheckoutErr(w http.ResponseWriter, r *http.Request, err error) {
	var blocking *BlockingLineError
	var unavail *StockUnavailableError

	switch {
	case errors.Is(err, ErrCartEmpty):
		httpx.WriteError(w, httpx.CodeConflict, "Your cart is empty.")

	case errors.As(err, &blocking):
		httpx.WriteError(w, httpx.CodeConflict,
			blocking.Name+" cannot be fulfilled right now. Please review your cart.")

	case errors.Is(err, ErrNoSavedAddress):
		httpx.WriteError(w, httpx.CodeValidation,
			"Please add a shipping address before placing an order.")

	case errors.Is(err, ErrAddressNotFound):
		httpx.WriteError(w, httpx.CodeNotFound, "Shipping address not found.")

	case errors.Is(err, ErrPendingOrderExists):
		httpx.WriteError(w, httpx.CodeConflict,
			"You already have a checkout in progress. Please try again in a moment.")

	case errors.As(err, &unavail):
		httpx.WriteError(w, httpx.CodeConflict,
			unavail.Name+" is no longer available in the requested quantity.")

	case errors.Is(err, ErrGateway):
		httpx.WriteError(w, httpx.CodeGatewayError,
			"Payment gateway error. Please try again.")

	default:
		h.logger.ErrorContext(r.Context(), "order: open checkout",
			slog.Any("error", err),
			slog.String("request_id", httpx.RequestIDFrom(r.Context())),
		)
		httpx.WriteError(w, httpx.CodeInternal, "Something went wrong on our side.")
	}
}

func toOrderDTO(o Order) orderDTO {
	return orderDTO{
		OrderID:         o.OrderID,
		Status:          string(o.Status),
		CreatedAt:       o.CreatedAt,
		ExpiresAt:       o.ExpiresAt,
		Lines:           toOrderLineDTOs(o.Lines),
		Totals:          toTotalsDTO(o.Totals),
		ShippingAddress: o.ShippingAddress,
	}
}

func toPlacedOrderDTO(o Order) placedOrderDTO {
	dto := placedOrderDTO{
		OrderID:         o.OrderID,
		Status:          string(o.Status),
		CreatedAt:       o.CreatedAt,
		ExpiresAt:       o.ExpiresAt,
		PlacedAt:        o.PlacedAt,
		Lines:           toOrderLineDTOs(o.Lines),
		Totals:          toTotalsDTO(o.Totals),
		ShippingAddress: o.ShippingAddress,
	}
	if o.Status == StatusPlaced {
		dto.EtaText = etaText
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

func toOrderLineDTOs(lines []Line) []orderLineDTO {
	out := make([]orderLineDTO, len(lines))
	for i, l := range lines {
		out[i] = orderLineDTO{
			ProductID: string(l.ProductID),
			Name:      l.Name,
			Form:      string(l.Form),
			Grad:      l.Grad,
			UnitPrice: l.UnitPrice,
			UnitMrp:   l.UnitMrp,
			Qty:       l.Qty,
			LineTotal: l.LineTotal,
		}
	}
	return out
}

func toTotalsDTO(t Totals) orderTotalsDTO {
	return orderTotalsDTO(t) // same fields, same types — only tags differ
}

// optStr converts an empty string to nil so the JSON field renders as null.
func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
