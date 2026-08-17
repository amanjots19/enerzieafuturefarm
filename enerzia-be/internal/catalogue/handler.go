package catalogue

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/enerzia/enerzia-be/internal/httpx"
)

// Handler serves the catalogue endpoints defined in roadmap.md §Catalogue.
// It does HTTP only: decode, delegate, encode.
type Handler struct {
	service *Service
	logger  *slog.Logger
}

// NewHandler builds the catalogue HTTP handler.
func NewHandler(service *Service, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

// Register mounts the catalogue routes on r, which is expected to be the
// /api/v1 subrouter.
func (h *Handler) Register(r *mux.Router) {
	r.HandleFunc("/products", h.listProducts).Methods(http.MethodGet)
	r.HandleFunc("/products/{id}", h.getProduct).Methods(http.MethodGet)
	r.HandleFunc("/content/trust", h.getTrust).Methods(http.MethodGet)
}

/* ---------------------------------------------------------------- wire DTOs */

// The API shape is deliberately separate from the stored shape: discountPercent
// and soldOut are computed rather than stored.

type productDTO struct {
	ID              string  `json:"id"`
	Family          string  `json:"family"`
	Form            string  `json:"form"`
	Name            string  `json:"name"`
	Stat            string  `json:"stat"`
	Stat2           string  `json:"stat2"`
	Blurb           string  `json:"blurb"`
	Grad            string  `json:"grad"`
	Images          []Image `json:"images"`
	MRP             int64   `json:"mrp"`
	Price           int64   `json:"price"`
	DiscountPercent int     `json:"discountPercent"`
	Stock           int     `json:"stock"`
	SoldOut         bool    `json:"soldOut"`
}

type siblingDTO struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Stat2   string `json:"stat2"`
	MRP     int64  `json:"mrp"`
	Price   int64  `json:"price"`
	SoldOut bool   `json:"soldOut"`
}

type listResponse struct {
	Products []productDTO `json:"products"`
}

type detailResponse struct {
	Product   productDTO   `json:"product"`
	Siblings  []siblingDTO `json:"siblings"`
	Rating    Rating       `json:"rating"`
	Badges    []Badge      `json:"badges"`
	Nutrition Nutrition    `json:"nutrition"`
}

type trustResponse struct {
	Tiles []TrustTile `json:"tiles"`
}

func toProductDTO(p Product) productDTO {
	return productDTO{
		ID:              string(p.ID),
		Family:          string(p.Family),
		Form:            string(p.Form),
		Name:            p.Name,
		Stat:            p.Stat,
		Stat2:           p.Stat2,
		Blurb:           p.Blurb,
		Grad:            p.Grad,
		Images:          orEmpty(p.Images),
		MRP:             p.MRP,
		Price:           p.Price,
		DiscountPercent: p.DiscountPercent(),
		Stock:           p.Stock,
		SoldOut:         p.SoldOut(),
	}
}

func toSiblingDTOs(siblings []Sibling) []siblingDTO {
	out := make([]siblingDTO, 0, len(siblings))
	for _, s := range siblings {
		out = append(out, siblingDTO{
			ID:      string(s.ID),
			Name:    s.Name,
			Stat2:   s.Stat2,
			MRP:     s.MRP,
			Price:   s.Price,
			SoldOut: s.SoldOut,
		})
	}
	return out
}

/* ----------------------------------------------------------------- handlers */

func (h *Handler) listProducts(w http.ResponseWriter, r *http.Request) {
	products, err := h.service.Products(r.Context(), r.URL.Query().Get("form"))
	if err != nil {
		if errors.Is(err, ErrUnknownForm) {
			httpx.WriteError(w, httpx.CodeBadRequest, "That product type does not exist.")
			return
		}
		h.fail(r, w, "list products", err)
		return
	}

	dtos := make([]productDTO, 0, len(products))
	for _, p := range products {
		dtos = append(dtos, toProductDTO(p))
	}
	httpx.WriteData(w, http.StatusOK, listResponse{Products: dtos})
}

func (h *Handler) getProduct(w http.ResponseWriter, r *http.Request) {
	id := ID(mux.Vars(r)["id"])

	detail, err := h.service.Detail(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrProductNotFound) {
			httpx.WriteError(w, httpx.CodeNotFound, "That product does not exist.")
			return
		}
		h.fail(r, w, "get product", err)
		return
	}

	httpx.WriteData(w, http.StatusOK, detailResponse{
		Product:   toProductDTO(detail.Product),
		Siblings:  toSiblingDTOs(detail.Siblings),
		Rating:    detail.Product.Rating,
		Badges:    orEmpty(detail.Product.Badges),
		Nutrition: normaliseNutrition(detail.Product.Nutrition),
	})
}

func (h *Handler) getTrust(w http.ResponseWriter, r *http.Request) {
	tiles, err := h.service.TrustTiles(r.Context())
	if err != nil {
		h.fail(r, w, "get trust tiles", err)
		return
	}
	httpx.WriteData(w, http.StatusOK, trustResponse{Tiles: orEmpty(tiles)})
}

// fail logs the underlying cause and returns a generic 500. Database and
// driver detail is never sent to the client.
func (h *Handler) fail(r *http.Request, w http.ResponseWriter, op string, err error) {
	h.logger.ErrorContext(r.Context(), "catalogue: "+op,
		slog.Any("error", err),
		slog.String("request_id", httpx.RequestIDFrom(r.Context())),
	)
	httpx.WriteError(w, httpx.CodeInternal, "Something went wrong on our side.")
}

// orEmpty keeps a nil slice from serialising as null.
func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// normaliseNutrition ensures Rows is never nil so it serialises as [] not null.
func normaliseNutrition(n Nutrition) Nutrition {
	n.Rows = orEmpty(n.Rows)
	return n
}
