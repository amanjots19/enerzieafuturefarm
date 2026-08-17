package catalogue

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/enerzia/enerzia-be/internal/admin"
	"github.com/enerzia/enerzia-be/internal/httpx"
)

// AdminHandler serves the admin catalogue endpoints defined in roadmap.md §Admin.
type AdminHandler struct {
	service *AdminService
	parser  admin.TokenParser
	logger  *slog.Logger
}

// NewAdminHandler builds the admin catalogue HTTP handler.
func NewAdminHandler(service *AdminService, parser admin.TokenParser, logger *slog.Logger) *AdminHandler {
	return &AdminHandler{service: service, parser: parser, logger: logger}
}

// Register mounts the admin catalogue routes under /admin on r, which is
// expected to be the /api/v1 subrouter. All routes require a valid admin token.
func (h *AdminHandler) Register(r *mux.Router) {
	sub := r.PathPrefix("/admin").Subrouter()
	sub.Use(admin.Require(h.parser))
	sub.HandleFunc("/products", h.listProducts).Methods(http.MethodGet)
	sub.HandleFunc("/products", h.createProduct).Methods(http.MethodPost)
	sub.HandleFunc("/products/{id}", h.getProduct).Methods(http.MethodGet)
	sub.HandleFunc("/products/{id}", h.updateProduct).Methods(http.MethodPut)
	sub.HandleFunc("/products/{id}", h.retireProduct).Methods(http.MethodDelete)
}

/* ---------------------------------------------------------------- wire DTOs */

// adminProductDTO is the full admin view of a product, including the fields
// the shopper's view omits (active, position, rating, badges, nutrition on the
// list) and the computed fields (discountPercent, soldOut).
type adminProductDTO struct {
	ID              string    `json:"id"`
	Family          string    `json:"family"`
	Form            string    `json:"form"`
	Name            string    `json:"name"`
	Stat            string    `json:"stat"`
	Stat2           string    `json:"stat2"`
	Blurb           string    `json:"blurb"`
	Grad            string    `json:"grad"`
	Position        int       `json:"position"`
	Images          []Image   `json:"images"`
	MRP             int64     `json:"mrp"`
	Price           int64     `json:"price"`
	DiscountPercent int       `json:"discountPercent"`
	Stock           int       `json:"stock"`
	SoldOut         bool      `json:"soldOut"`
	Active          bool      `json:"active"`
	Rating          Rating    `json:"rating"`
	Badges          []Badge   `json:"badges"`
	Nutrition       Nutrition `json:"nutrition"`
}

func toAdminProductDTO(p Product) adminProductDTO {
	return adminProductDTO{
		ID:              string(p.ID),
		Family:          string(p.Family),
		Form:            string(p.Form),
		Name:            p.Name,
		Stat:            p.Stat,
		Stat2:           p.Stat2,
		Blurb:           p.Blurb,
		Grad:            p.Grad,
		Position:        p.Position,
		Images:          orEmpty(p.Images),
		MRP:             p.MRP,
		Price:           p.Price,
		DiscountPercent: p.DiscountPercent(),
		Stock:           p.Stock,
		SoldOut:         p.SoldOut(),
		Active:          p.Active,
		Rating:          p.Rating,
		Badges:          orEmpty(p.Badges),
		Nutrition:       normaliseNutrition(p.Nutrition),
	}
}

func toAdminProductDTOs(products []Product) []adminProductDTO {
	out := make([]adminProductDTO, 0, len(products))
	for _, p := range products {
		out = append(out, toAdminProductDTO(p))
	}
	return out
}

// adminWriteBody is the request body for POST and PUT. The computed fields
// discountPercent and soldOut are included as pointers so their presence in a
// write payload can be detected and rejected with 422.
type adminWriteBody struct {
	ID        string    `json:"id"`
	Family    string    `json:"family"`
	Form      string    `json:"form"`
	Name      string    `json:"name"`
	Stat      string    `json:"stat"`
	Stat2     string    `json:"stat2"`
	Blurb     string    `json:"blurb"`
	Grad      string    `json:"grad"`
	Position  int       `json:"position"`
	Images    []Image   `json:"images"`
	MRP       int64     `json:"mrp"`
	Price     int64     `json:"price"`
	Stock     int       `json:"stock"`
	Active    bool      `json:"active"`
	Rating    Rating    `json:"rating"`
	Badges    []Badge   `json:"badges"`
	Nutrition Nutrition `json:"nutrition"`

	// Computed — presence in a write body is rejected with 422.
	DiscountPercent *int  `json:"discountPercent"`
	SoldOut         *bool `json:"soldOut"`
}

func (b adminWriteBody) toProduct(id ID) Product {
	return Product{
		ID:        id,
		Family:    Family(b.Family),
		Form:      Form(b.Form),
		Name:      b.Name,
		Stat:      b.Stat,
		Stat2:     b.Stat2,
		Blurb:     b.Blurb,
		Grad:      b.Grad,
		Position:  b.Position,
		Images:    b.Images,
		MRP:       b.MRP,
		Price:     b.Price,
		Stock:     b.Stock,
		Active:    b.Active,
		Rating:    b.Rating,
		Badges:    b.Badges,
		Nutrition: b.Nutrition,
	}
}

/* ----------------------------------------------------------------- handlers */

func (h *AdminHandler) listProducts(w http.ResponseWriter, r *http.Request) {
	products, err := h.service.Products(r.Context())
	if err != nil {
		h.fail(r, w, "list products", err)
		return
	}
	type listResp struct {
		Products []adminProductDTO `json:"products"`
	}
	httpx.WriteData(w, http.StatusOK, listResp{Products: toAdminProductDTOs(products)})
}

func (h *AdminHandler) createProduct(w http.ResponseWriter, r *http.Request) {
	var body adminWriteBody
	if !decodeAdminProductJSON(w, r, &body) {
		return
	}
	if !rejectComputedFields(w, &body) {
		return
	}
	p := body.toProduct(ID(body.ID))
	if problem, ok := ValidateForWrite(p); !ok {
		httpx.WriteFieldErrors(w, []httpx.FieldError{{Field: problem.Field, Message: problem.Message}})
		return
	}
	if err := h.service.Create(r.Context(), p); err != nil {
		if errors.Is(err, ErrDuplicateProduct) {
			httpx.WriteError(w, httpx.CodeConflict, "A product with that id already exists.")
			return
		}
		h.fail(r, w, "create product", err)
		return
	}
	httpx.WriteData(w, http.StatusCreated, toAdminProductDTO(p))
}

func (h *AdminHandler) getProduct(w http.ResponseWriter, r *http.Request) {
	id := ID(mux.Vars(r)["id"])
	p, err := h.service.Product(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrProductNotFound) {
			httpx.WriteError(w, httpx.CodeNotFound, "That product does not exist.")
			return
		}
		h.fail(r, w, "get product", err)
		return
	}
	httpx.WriteData(w, http.StatusOK, toAdminProductDTO(p))
}

func (h *AdminHandler) updateProduct(w http.ResponseWriter, r *http.Request) {
	pathID := ID(mux.Vars(r)["id"])

	var body adminWriteBody
	if !decodeAdminProductJSON(w, r, &body) {
		return
	}
	if !rejectComputedFields(w, &body) {
		return
	}
	// Body id must match the path or be absent — renaming would orphan cart and
	// order lines that reference the current id.
	if body.ID != "" && body.ID != string(pathID) {
		httpx.WriteFieldErrors(w, []httpx.FieldError{
			{Field: "id", Message: "A product id cannot be changed."},
		})
		return
	}
	p := body.toProduct(pathID)
	if problem, ok := ValidateForWrite(p); !ok {
		httpx.WriteFieldErrors(w, []httpx.FieldError{{Field: problem.Field, Message: problem.Message}})
		return
	}
	if err := h.service.Update(r.Context(), p); err != nil {
		if errors.Is(err, ErrProductNotFound) {
			httpx.WriteError(w, httpx.CodeNotFound, "That product does not exist.")
			return
		}
		h.fail(r, w, "update product", err)
		return
	}
	httpx.WriteData(w, http.StatusOK, toAdminProductDTO(p))
}

func (h *AdminHandler) retireProduct(w http.ResponseWriter, r *http.Request) {
	id := ID(mux.Vars(r)["id"])
	if err := h.service.Retire(r.Context(), id); err != nil {
		if errors.Is(err, ErrProductNotFound) {
			httpx.WriteError(w, httpx.CodeNotFound, "That product does not exist.")
			return
		}
		h.fail(r, w, "retire product", err)
		return
	}
	p, err := h.service.Product(r.Context(), id)
	if err != nil {
		h.fail(r, w, "get retired product", err)
		return
	}
	httpx.WriteData(w, http.StatusOK, toAdminProductDTO(p))
}

/* ------------------------------------------------------------------ helpers */

// decodeAdminProductJSON reads a JSON body rejecting unknown fields (so a typo
// in a client payload surfaces as a 400 rather than being silently dropped).
func decodeAdminProductJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	const maxBodyBytes = 64 << 10 // 64 KiB — generous for a product body
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		httpx.WriteError(w, httpx.CodeBadRequest, "The request body could not be read.")
		return false
	}
	return true
}

// rejectComputedFields returns false and writes a 422 when discountPercent or
// soldOut are present in the request body. They are outputs, not inputs —
// accepting them would create a second source of truth that disagrees with the
// values the server computes.
func rejectComputedFields(w http.ResponseWriter, body *adminWriteBody) bool {
	if body.DiscountPercent != nil || body.SoldOut != nil {
		httpx.WriteFieldErrors(w, []httpx.FieldError{
			{Field: "discountPercent", Message: "discountPercent and soldOut are computed, not stored."},
		})
		return false
	}
	return true
}

// fail logs the underlying cause and returns a generic 500.
func (h *AdminHandler) fail(r *http.Request, w http.ResponseWriter, op string, err error) {
	h.logger.ErrorContext(r.Context(), "catalogue admin: "+op,
		slog.Any("error", err),
		slog.String("request_id", httpx.RequestIDFrom(r.Context())),
	)
	httpx.WriteError(w, httpx.CodeInternal, "Something went wrong on our side.")
}
