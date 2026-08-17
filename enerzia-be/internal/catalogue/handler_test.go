package catalogue_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/enerzia/enerzia-be/internal/catalogue"
	"github.com/enerzia/enerzia-be/internal/config"
	"github.com/enerzia/enerzia-be/internal/server"
)

// stubReader is an in-memory catalogue.Reader, so handler tests exercise
// routing and encoding without a database in the way.
type stubReader struct {
	products []catalogue.Product
	tiles    []catalogue.TrustTile

	listErr  error
	getErr   error
	trustErr error

	gotForm     catalogue.Form
	gotFiltered bool
	gotID       catalogue.ID
}

func (s *stubReader) List(_ context.Context, form catalogue.Form, filtered bool) ([]catalogue.Product, error) {
	s.gotForm, s.gotFiltered = form, filtered
	if s.listErr != nil {
		return nil, s.listErr
	}
	var out []catalogue.Product
	for _, p := range s.products {
		if !p.Active {
			continue
		}
		if !filtered || p.Form == form {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *stubReader) Siblings(_ context.Context, family catalogue.Family, exclude catalogue.ID) ([]catalogue.Product, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	var out []catalogue.Product
	for _, p := range s.products {
		if p.Family == family && p.ID != exclude && p.Active {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *stubReader) Get(_ context.Context, id catalogue.ID) (catalogue.Product, error) {
	s.gotID = id
	if s.getErr != nil {
		return catalogue.Product{}, s.getErr
	}
	for _, p := range s.products {
		if p.ID == id {
			return p, nil
		}
	}
	return catalogue.Product{}, catalogue.ErrProductNotFound
}

func (s *stubReader) TrustTiles(context.Context) ([]catalogue.TrustTile, error) {
	if s.trustErr != nil {
		return nil, s.trustErr
	}
	return s.tiles, nil
}

// newAPI wires the stub through the real service, handler and router.
func newAPI(t *testing.T, reader *stubReader) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	return server.New(server.Deps{
		Config:    config.Config{},
		Mongo:     stubPinger{},
		Catalogue: catalogue.NewHandler(catalogue.NewService(reader), logger),
		Logger:    logger,
		Version:   "test",
		Started:   time.Now(),
	})
}

type stubPinger struct{}

func (stubPinger) Ping(context.Context) error { return nil }

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func decodeInto(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("body is not JSON: %v (%s)", err, rec.Body.String())
	}
}

func errorCodeOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	decodeInto(t, rec, &body)
	return body.Error.Code
}

// seeded returns a reader holding the real catalogue.
func seeded() *stubReader {
	return &stubReader{products: catalogue.SeedProducts(), tiles: catalogue.SeedTrustTiles()}
}

/* ------------------------------------------------ GET /api/v1/products */

func TestListProductsReturnsTheCatalogue(t *testing.T) {
	rec := get(t, newAPI(t, seeded()), "/api/v1/products")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}

	var body struct {
		Data struct {
			Products []struct {
				ID              string `json:"id"`
				Family          string `json:"family"`
				Form            string `json:"form"`
				Name            string `json:"name"`
				MRP             int64  `json:"mrp"`
				Price           int64  `json:"price"`
				DiscountPercent int    `json:"discountPercent"`
				Stock           int    `json:"stock"`
				SoldOut         bool   `json:"soldOut"`
			} `json:"products"`
		} `json:"data"`
	}
	decodeInto(t, rec, &body)

	// Every size is its own card now: nine, not four.
	if len(body.Data.Products) != 9 {
		t.Fatalf("returned %d products, want 9", len(body.Data.Products))
	}

	var tablets120 bool
	for _, p := range body.Data.Products {
		if p.ID != "tablets-120" {
			continue
		}
		tablets120 = true
		if p.Family != "tablets" {
			t.Errorf("family = %q, want tablets", p.Family)
		}
		// Prices go out as paise, and the discount is computed by the server.
		if p.Price != 38000 || p.MRP != 47000 || p.DiscountPercent != 19 {
			t.Errorf("tablets-120 = %+v, want price 38000, mrp 47000, 19%% off", p)
		}
		if p.SoldOut || p.Stock <= 0 {
			t.Errorf("seeded stock should not read as sold out: %+v", p)
		}
	}
	if !tablets120 {
		t.Error("tablets-120 is missing from the grid")
	}
}

func TestListProductsFiltersByForm(t *testing.T) {
	reader := seeded()
	rec := get(t, newAPI(t, reader), "/api/v1/products?form=Powder")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if reader.gotForm != catalogue.FormPowder || !reader.gotFiltered {
		t.Errorf("repository received form=%q filtered=%v", reader.gotForm, reader.gotFiltered)
	}

	var body struct {
		Data struct {
			Products []struct {
				ID string `json:"id"`
			} `json:"products"`
		} `json:"data"`
	}
	decodeInto(t, rec, &body)

	// Two powder tubs plus two refill pouches, all form Powder.
	if len(body.Data.Products) != 4 {
		t.Fatalf("returned %d products, want 4", len(body.Data.Products))
	}
}

func TestListProductsTreatsAllAsUnfiltered(t *testing.T) {
	for _, q := range []string{"", "?form=", "?form=all"} {
		t.Run("query"+q, func(t *testing.T) {
			reader := seeded()
			rec := get(t, newAPI(t, reader), "/api/v1/products"+q)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if reader.gotFiltered {
				t.Error("repository was asked to filter")
			}
		})
	}
}

func TestListProductsRejectsAnUnknownForm(t *testing.T) {
	// Silently returning everything would look like an empty-grid bug.
	rec := get(t, newAPI(t, seeded()), "/api/v1/products?form=Capsules")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if got := errorCodeOf(t, rec); got != "BAD_REQUEST" {
		t.Errorf("error.code = %q, want BAD_REQUEST", got)
	}
}

func TestListProductsReturnsAnEmptyArrayNotNull(t *testing.T) {
	rec := get(t, newAPI(t, &stubReader{}), "/api/v1/products")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"products":[]`) {
		t.Errorf("body = %s, want products to serialise as []", rec.Body.String())
	}
}

func TestListProductsHidesDatabaseFailures(t *testing.T) {
	reader := seeded()
	reader.listErr = errors.New("connection to cluster0-shard-00.mongodb.net refused")

	rec := get(t, newAPI(t, reader), "/api/v1/products")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "mongodb.net") {
		t.Errorf("response leaks infrastructure detail: %s", rec.Body.String())
	}
}

func TestListProductsOmitsDetailContent(t *testing.T) {
	// The grid must not ship the PDP payload for every card.
	rec := get(t, newAPI(t, seeded()), "/api/v1/products")

	for _, field := range []string{"nutrition", "badges", "rating", "siblings"} {
		if strings.Contains(rec.Body.String(), `"`+field+`"`) {
			t.Errorf("list response includes %q, which belongs to the detail endpoint", field)
		}
	}
}

/* --------------------------------------------- GET /api/v1/products/{id} */

func TestGetProductReturnsDetailContent(t *testing.T) {
	rec := get(t, newAPI(t, seeded()), "/api/v1/products/tablets-120")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}

	var body struct {
		Data struct {
			Product struct {
				ID    string `json:"id"`
				Blurb string `json:"blurb"`
			} `json:"product"`
			Siblings []struct {
				ID string `json:"id"`
			} `json:"siblings"`
			Rating struct {
				Score float64 `json:"score"`
				Count int     `json:"count"`
			} `json:"rating"`
			Badges []struct {
				Title string `json:"title"`
			} `json:"badges"`
			Nutrition struct {
				ServingSize string `json:"servingSize"`
				Rows        []struct {
					Key   string `json:"key"`
					Value string `json:"value"`
				} `json:"rows"`
			} `json:"nutrition"`
		} `json:"data"`
	}
	decodeInto(t, rec, &body)

	if body.Data.Product.ID != "tablets-120" {
		t.Errorf("product.id = %q, want tablets-120", body.Data.Product.ID)
	}
	// The other two tablet sizes, so the page can cross-link them.
	if len(body.Data.Siblings) != 2 {
		t.Errorf("%d siblings, want the other two tablet sizes", len(body.Data.Siblings))
	}
	for _, sib := range body.Data.Siblings {
		if sib.ID == "tablets-120" {
			t.Error("a product is listed as its own sibling")
		}
	}
	if body.Data.Product.Blurb == "" {
		t.Error("product.blurb is empty")
	}
	if body.Data.Rating.Score != 4.8 || body.Data.Rating.Count != 312 {
		t.Errorf("rating = %+v, want 4.8 / 312", body.Data.Rating)
	}
	if len(body.Data.Badges) != 4 {
		t.Errorf("%d badges, want 4", len(body.Data.Badges))
	}
	if body.Data.Nutrition.ServingSize != "5 g" || len(body.Data.Nutrition.Rows) != 5 {
		t.Errorf("nutrition = %+v, want 5 g with 5 rows", body.Data.Nutrition)
	}
}

func TestGetProductReturns404ForAnUnknownID(t *testing.T) {
	reader := seeded()
	rec := get(t, newAPI(t, reader), "/api/v1/products/capsules")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if got := errorCodeOf(t, rec); got != "NOT_FOUND" {
		t.Errorf("error.code = %q, want NOT_FOUND", got)
	}
	if reader.gotID != "capsules" {
		t.Errorf("repository received id %q, want capsules", reader.gotID)
	}
}

func TestGetProductHidesDatabaseFailures(t *testing.T) {
	reader := seeded()
	reader.getErr = errors.New("auth failed for user appuser")

	rec := get(t, newAPI(t, reader), "/api/v1/products/tablets-120")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "appuser") {
		t.Errorf("response leaks driver detail: %s", rec.Body.String())
	}
}

func TestGetProductBadgesSerialiseAsArrayWhenAbsent(t *testing.T) {
	bare := catalogue.SeedProducts()[0]
	bare.Badges = nil
	rec := get(t, newAPI(t, &stubReader{products: []catalogue.Product{bare}}), "/api/v1/products/"+string(bare.ID))

	if !strings.Contains(rec.Body.String(), `"badges":[]`) {
		t.Errorf("body = %s, want badges to serialise as []", rec.Body.String())
	}
}

func TestGetProductNutritionRowsSerialiseAsArrayWhenAbsent(t *testing.T) {
	// A product with no nutrition rows must return "rows":[] not "rows":null.
	// Assert on raw JSON text — Go's JSON decoder maps both [] and null to a nil
	// slice, so a struct-level check would silently miss the bug.
	p := tabletsProduct()
	p.Nutrition = catalogue.Nutrition{ServingSize: "", Rows: nil}
	rec := get(t, newAPI(t, &stubReader{products: []catalogue.Product{p}}), "/api/v1/products/"+string(p.ID))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"rows":[]`) {
		t.Errorf("body = %s, want nutrition.rows to serialise as []", rec.Body.String())
	}
}

/* ------------------------------------------- GET /api/v1/content/trust */

func TestGetTrustReturnsTheTiles(t *testing.T) {
	rec := get(t, newAPI(t, seeded()), "/api/v1/content/trust")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}

	var body struct {
		Data struct {
			Tiles []struct {
				Big  string `json:"big"`
				Body string `json:"body"`
			} `json:"tiles"`
		} `json:"data"`
	}
	decodeInto(t, rec, &body)

	if len(body.Data.Tiles) != 4 {
		t.Fatalf("%d tiles, want 4", len(body.Data.Tiles))
	}
	if body.Data.Tiles[0].Big != "62%+" || body.Data.Tiles[0].Body == "" {
		t.Errorf("first tile = %+v", body.Data.Tiles[0])
	}
}

func TestGetTrustReturnsAnEmptyArrayWhenUnseeded(t *testing.T) {
	rec := get(t, newAPI(t, &stubReader{}), "/api/v1/content/trust")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"tiles":[]`) {
		t.Errorf("body = %s, want tiles to serialise as []", rec.Body.String())
	}
}

func TestGetTrustHidesDatabaseFailures(t *testing.T) {
	reader := seeded()
	reader.trustErr = errors.New("connection reset by peer")

	rec := get(t, newAPI(t, reader), "/api/v1/content/trust")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "connection reset") {
		t.Errorf("response leaks internals: %s", rec.Body.String())
	}
}

/* ---------------------------------------------- images serialisation */

func TestListProductsImagesSerialisesAsEmptyArray(t *testing.T) {
	// The seeded products carry no photographs; the field must still come out as
	// [] so the client never has to guard against null.
	rec := get(t, newAPI(t, seeded()), "/api/v1/products")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"images":[]`) {
		t.Errorf("body = %s, want images to serialise as [] for products without photographs", rec.Body.String())
	}
}

func TestGetProductImagesSerialisesAsEmptyArray(t *testing.T) {
	rec := get(t, newAPI(t, seeded()), "/api/v1/products/tablets-120")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"images":[]`) {
		t.Errorf("body = %s, want images to serialise as [] for a product without photographs", rec.Body.String())
	}
}

func TestGetProductRoundTripsImages(t *testing.T) {
	// A product WITH images must deliver url, publicId and alt faithfully.
	p := tabletsProduct()
	p.Images = []catalogue.Image{
		{
			URL:      "https://res.cloudinary.com/enerzia/image/upload/v1/products/abc.jpg",
			PublicID: "enerzia/products/abc",
			Alt:      "Tablet jar, front",
		},
	}
	reader := &stubReader{products: []catalogue.Product{p}}

	rec := get(t, newAPI(t, reader), "/api/v1/products/tablets-120")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}

	var body struct {
		Data struct {
			Product struct {
				Images []struct {
					URL      string `json:"url"`
					PublicID string `json:"publicId"`
					Alt      string `json:"alt"`
				} `json:"images"`
			} `json:"product"`
		} `json:"data"`
	}
	decodeInto(t, rec, &body)

	imgs := body.Data.Product.Images
	if len(imgs) != 1 {
		t.Fatalf("images count = %d, want 1", len(imgs))
	}
	if imgs[0].URL != "https://res.cloudinary.com/enerzia/image/upload/v1/products/abc.jpg" {
		t.Errorf("url = %q", imgs[0].URL)
	}
	if imgs[0].PublicID != "enerzia/products/abc" {
		t.Errorf("publicId = %q", imgs[0].PublicID)
	}
	if imgs[0].Alt != "Tablet jar, front" {
		t.Errorf("alt = %q", imgs[0].Alt)
	}
}

/* --------------------------------------------------------- routing rules */

func TestCatalogueRoutesRejectWrongMethods(t *testing.T) {
	h := newAPI(t, seeded())
	for _, path := range []string{"/api/v1/products", "/api/v1/products/tablets-120", "/api/v1/content/trust"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))

			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("POST %s status = %d, want 405", path, rec.Code)
			}
		})
	}
}

func TestCatalogueResponsesAreJSON(t *testing.T) {
	h := newAPI(t, seeded())
	for _, path := range []string{"/api/v1/products", "/api/v1/products/tablets-120", "/api/v1/content/trust"} {
		rec := get(t, h, path)
		if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
			t.Errorf("GET %s Content-Type = %q", path, ct)
		}
	}
}
