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

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/enerzia/enerzia-be/internal/admin"
	"github.com/enerzia/enerzia-be/internal/auth"
	"github.com/enerzia/enerzia-be/internal/catalogue"
	"github.com/enerzia/enerzia-be/internal/config"
	"github.com/enerzia/enerzia-be/internal/server"
)

// adminCatalogueSecret is the shared HS256 secret used by the admin token
// issuer in these tests. It is intentionally different from the shopper secret.
var adminCatalogueSecret = []byte("admin-catalogue-test-secret-at-least-32-chars!!")

// testTokenParser adapts *admin.TokenIssuer to the admin.TokenParser interface.
// admin.Service wraps a TokenIssuer for production use; tests bypass the full
// service because they only need token verification, not bcrypt login.
type testTokenParser struct{ issuer *admin.TokenIssuer }

func (t testTokenParser) ParseToken(tok string) (admin.Claims, error) {
	return t.issuer.Parse(tok)
}

// stubWriter is an in-memory Writer so handler tests exercise routing and
// encoding without a database.
type stubWriter struct {
	products []catalogue.Product

	listAllErr error
	getErr     error
	createErr  error
	updateErr  error
	retireErr  error
}

func (s *stubWriter) ListAll(_ context.Context) ([]catalogue.Product, error) {
	if s.listAllErr != nil {
		return nil, s.listAllErr
	}
	return s.products, nil
}

func (s *stubWriter) Get(_ context.Context, id catalogue.ID) (catalogue.Product, error) {
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

func (s *stubWriter) Create(_ context.Context, p catalogue.Product) error {
	if s.createErr != nil {
		return s.createErr
	}
	s.products = append(s.products, p)
	return nil
}

func (s *stubWriter) Update(_ context.Context, p catalogue.Product) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	for i, existing := range s.products {
		if existing.ID == p.ID {
			s.products[i] = p
			return nil
		}
	}
	return catalogue.ErrProductNotFound
}

func (s *stubWriter) Retire(_ context.Context, id catalogue.ID) error {
	if s.retireErr != nil {
		return s.retireErr
	}
	for i, p := range s.products {
		if p.ID == id {
			s.products[i].Active = false
			return nil
		}
	}
	return catalogue.ErrProductNotFound
}

// newAdminCatalogueAPI wires the stub writer through the real service, handler
// and router so routing, middleware, and encoding are all exercised.
func newAdminCatalogueAPI(t *testing.T, w *stubWriter) (http.Handler, *admin.TokenIssuer) {
	t.Helper()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	tokens := admin.NewTokenIssuer(adminCatalogueSecret, admin.TokenTTL)
	svc := catalogue.NewAdminService(w)
	ah := catalogue.NewAdminHandler(svc, testTokenParser{tokens}, logger)
	h := server.New(server.Deps{
		Config:         config.Config{},
		Mongo:          stubPinger{},
		AdminCatalogue: ah,
		Logger:         logger,
		Version:        "test",
		Started:        time.Now(),
	})
	return h, tokens
}

// issueAdminToken returns a signed admin token.
func issueAdminToken(t *testing.T, issuer *admin.TokenIssuer) string {
	t.Helper()
	tok, _, err := issuer.Issue("ops@enerzia.in")
	if err != nil {
		t.Fatalf("issue admin token: %v", err)
	}
	return tok
}

// issueShopperToken returns a shopper token whose issuer ("enerzia-api") will
// be rejected by the admin.Require middleware.
func issueShopperToken(t *testing.T) string {
	t.Helper()
	shopperIssuer := auth.NewTokenIssuer(adminCatalogueSecret, 30*24*time.Hour)
	tok, _, err := shopperIssuer.Issue(auth.User{ID: bson.NewObjectID()})
	if err != nil {
		t.Fatalf("issue shopper token: %v", err)
	}
	return tok
}

func doAdminCatalogue(
	t *testing.T, h http.Handler,
	method, path, body, bearer string,
) *httptest.ResponseRecorder {
	t.Helper()
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func catalogueAdminErrorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("body is not valid JSON: %v (%s)", err, rec.Body.String())
	}
	return env.Error.Code
}

func catalogueAdminErrorMessage(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("body is not valid JSON: %v (%s)", err, rec.Body.String())
	}
	return env.Error.Message
}

// activeProduct returns a canonical product for use in test stubs.
func activeProduct() catalogue.Product {
	return catalogue.Product{
		ID:       "tablets-120",
		Family:   "tablets",
		Form:     catalogue.FormTablets,
		Name:     "Spirulina Tablets 120",
		MRP:      47000,
		Price:    38000,
		Stock:    100,
		Position: 1,
		Active:   true,
		Rating:   catalogue.Rating{Score: 4.8, Count: 312},
	}
}

// validProductJSON returns a JSON body for a valid product write.
const validProductJSON = `{
	"id": "tablets-120",
	"family": "tablets",
	"form": "Tablets",
	"name": "Spirulina Tablets 120",
	"mrp": 47000,
	"price": 38000,
	"stock": 100,
	"position": 1,
	"active": true,
	"rating": {"score": 4.8, "count": 312},
	"badges": [],
	"images": [],
	"nutrition": {}
}`

/* ------------------------------------------------ auth guard (all routes) */

func TestAdminCatalogueRequiresAuth(t *testing.T) {
	h, _ := newAdminCatalogueAPI(t, &stubWriter{products: []catalogue.Product{activeProduct()}})

	routes := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/v1/admin/products", ""},
		{http.MethodPost, "/api/v1/admin/products", validProductJSON},
		{http.MethodGet, "/api/v1/admin/products/tablets-120", ""},
		{http.MethodPut, "/api/v1/admin/products/tablets-120", validProductJSON},
		{http.MethodDelete, "/api/v1/admin/products/tablets-120", ""},
	}

	for _, r := range routes {
		t.Run(r.method+" "+r.path+" no-token", func(t *testing.T) {
			rec := doAdminCatalogue(t, h, r.method, r.path, r.body, "")
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", rec.Code)
			}
			if code := catalogueAdminErrorCode(t, rec); code != "UNAUTHORIZED" {
				t.Errorf("error.code = %q, want UNAUTHORIZED", code)
			}
		})

		t.Run(r.method+" "+r.path+" shopper-token", func(t *testing.T) {
			shopperTok := issueShopperToken(t)
			rec := doAdminCatalogue(t, h, r.method, r.path, r.body, shopperTok)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401 for shopper token", rec.Code)
			}
		})
	}
}

/* ------------------------------------------- GET /api/v1/admin/products */

func TestAdminListProductsReturnsAllIncludingRetired(t *testing.T) {
	active := activeProduct()
	retired := catalogue.Product{
		ID:     "tablets-60",
		Family: "tablets",
		Form:   catalogue.FormTablets,
		Name:   "Spirulina Tablets 60",
		MRP:    30000,
		Price:  25000,
		Stock:  0,
		Active: false, // retired
	}
	h, issuer := newAdminCatalogueAPI(t, &stubWriter{products: []catalogue.Product{active, retired}})
	tok := issueAdminToken(t, issuer)

	rec := doAdminCatalogue(t, h, http.MethodGet, "/api/v1/admin/products", "", tok)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var body struct {
		Data struct {
			Products []struct {
				ID     string `json:"id"`
				Active bool   `json:"active"`
			} `json:"products"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Data.Products) != 2 {
		t.Errorf("products count = %d, want 2 (admin sees retired products)", len(body.Data.Products))
	}
	var sawRetired bool
	for _, p := range body.Data.Products {
		if p.ID == "tablets-60" {
			sawRetired = true
			if p.Active {
				t.Error("retired product should have active=false in admin response")
			}
		}
	}
	if !sawRetired {
		t.Error("admin list must include the retired product")
	}
}

func TestAdminListProductsResponseIncludesComputedFields(t *testing.T) {
	h, issuer := newAdminCatalogueAPI(t, &stubWriter{products: []catalogue.Product{activeProduct()}})
	tok := issueAdminToken(t, issuer)

	rec := doAdminCatalogue(t, h, http.MethodGet, "/api/v1/admin/products", "", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Data struct {
			Products []map[string]any `json:"products"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Data.Products) == 0 {
		t.Fatal("no products in response")
	}
	p := body.Data.Products[0]
	if _, ok := p["discountPercent"]; !ok {
		t.Error("admin response must include discountPercent")
	}
	if _, ok := p["soldOut"]; !ok {
		t.Error("admin response must include soldOut")
	}
	if _, ok := p["active"]; !ok {
		t.Error("admin response must include active")
	}
}

func TestAdminListProductsWrapsErrors(t *testing.T) {
	w := &stubWriter{listAllErr: errors.New("db down")}
	h, issuer := newAdminCatalogueAPI(t, w)
	tok := issueAdminToken(t, issuer)

	rec := doAdminCatalogue(t, h, http.MethodGet, "/api/v1/admin/products", "", tok)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

/* -------------------------------------- POST /api/v1/admin/products */

func TestAdminCreateProductReturns201(t *testing.T) {
	h, issuer := newAdminCatalogueAPI(t, &stubWriter{})
	tok := issueAdminToken(t, issuer)

	rec := doAdminCatalogue(t, h, http.MethodPost, "/api/v1/admin/products", validProductJSON, tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (%s)", rec.Code, rec.Body.String())
	}
	var body struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.ID != "tablets-120" {
		t.Errorf("id = %q, want tablets-120", body.Data.ID)
	}
}

func TestAdminCreateProductRejects422OnValidationError(t *testing.T) {
	h, issuer := newAdminCatalogueAPI(t, &stubWriter{})
	tok := issueAdminToken(t, issuer)

	badBody := `{"id": "", "family": "tablets", "form": "Tablets", "name": "X", "mrp": 1000, "price": 900}`
	rec := doAdminCatalogue(t, h, http.MethodPost, "/api/v1/admin/products", badBody, tok)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", rec.Code)
	}
	if code := catalogueAdminErrorCode(t, rec); code != "VALIDATION_ERROR" {
		t.Errorf("error.code = %q, want VALIDATION_ERROR", code)
	}
}

func TestAdminCreateProductRejectsDiscountPercent(t *testing.T) {
	h, issuer := newAdminCatalogueAPI(t, &stubWriter{})
	tok := issueAdminToken(t, issuer)

	body := `{"id": "tablets-120", "family": "tablets", "form": "Tablets", "name": "X", "mrp": 1000, "price": 900, "discountPercent": 10}`
	rec := doAdminCatalogue(t, h, http.MethodPost, "/api/v1/admin/products", body, tok)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422 for discountPercent in write body", rec.Code)
	}
	if msg := catalogueAdminErrorMessage(t, rec); !strings.Contains(msg, "computed") {
		t.Errorf("message = %q, want it to mention computed", msg)
	}
}

func TestAdminCreateProductRejectsSoldOut(t *testing.T) {
	h, issuer := newAdminCatalogueAPI(t, &stubWriter{})
	tok := issueAdminToken(t, issuer)

	body := `{"id": "tablets-120", "family": "tablets", "form": "Tablets", "name": "X", "mrp": 1000, "price": 900, "soldOut": true}`
	rec := doAdminCatalogue(t, h, http.MethodPost, "/api/v1/admin/products", body, tok)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422 for soldOut in write body", rec.Code)
	}
}

func TestAdminCreateProductReturns409OnDuplicate(t *testing.T) {
	w := &stubWriter{createErr: catalogue.ErrDuplicateProduct}
	h, issuer := newAdminCatalogueAPI(t, w)
	tok := issueAdminToken(t, issuer)

	rec := doAdminCatalogue(t, h, http.MethodPost, "/api/v1/admin/products", validProductJSON, tok)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (%s)", rec.Code, rec.Body.String())
	}
	if code := catalogueAdminErrorCode(t, rec); code != "CONFLICT" {
		t.Errorf("error.code = %q, want CONFLICT", code)
	}
	if msg := catalogueAdminErrorMessage(t, rec); msg != "A product with that id already exists." {
		t.Errorf("message = %q, want exact conflict message", msg)
	}
}

func TestAdminCreateProductBadJSONReturns400(t *testing.T) {
	h, issuer := newAdminCatalogueAPI(t, &stubWriter{})
	tok := issueAdminToken(t, issuer)

	rec := doAdminCatalogue(t, h, http.MethodPost, "/api/v1/admin/products", "{not json", tok)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestAdminCreateProductUnknownFieldReturns400(t *testing.T) {
	h, issuer := newAdminCatalogueAPI(t, &stubWriter{})
	tok := issueAdminToken(t, issuer)

	body := `{"id": "tablets-120", "family": "tablets", "typo_field": "x"}`
	rec := doAdminCatalogue(t, h, http.MethodPost, "/api/v1/admin/products", body, tok)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for unknown field", rec.Code)
	}
}

/* -------------------------------- GET /api/v1/admin/products/{id} */

func TestAdminGetProductReturns200(t *testing.T) {
	h, issuer := newAdminCatalogueAPI(t, &stubWriter{products: []catalogue.Product{activeProduct()}})
	tok := issueAdminToken(t, issuer)

	rec := doAdminCatalogue(t, h, http.MethodGet, "/api/v1/admin/products/tablets-120", "", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var body struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.ID != "tablets-120" {
		t.Errorf("id = %q, want tablets-120", body.Data.ID)
	}
}

func TestAdminGetProductReturns404ForUnknownID(t *testing.T) {
	h, issuer := newAdminCatalogueAPI(t, &stubWriter{products: []catalogue.Product{}})
	tok := issueAdminToken(t, issuer)

	rec := doAdminCatalogue(t, h, http.MethodGet, "/api/v1/admin/products/does-not-exist", "", tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	if msg := catalogueAdminErrorMessage(t, rec); msg != "That product does not exist." {
		t.Errorf("message = %q, want exact not-found message", msg)
	}
}

func TestAdminGetProductIncludesRetiredProduct(t *testing.T) {
	// The admin detail endpoint must return a retired product, unlike the
	// shopper's GET /products/{id} which returns 404 for retired ones.
	retired := activeProduct()
	retired.Active = false

	h, issuer := newAdminCatalogueAPI(t, &stubWriter{products: []catalogue.Product{retired}})
	tok := issueAdminToken(t, issuer)

	rec := doAdminCatalogue(t, h, http.MethodGet, "/api/v1/admin/products/tablets-120", "", tok)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; admin must see retired products", rec.Code)
	}
}

func TestAdminGetProductNutritionRowsSerialiseAsArrayWhenAbsent(t *testing.T) {
	// A product with no nutrition rows must return "rows":[] not "rows":null.
	// Assert on raw JSON text — Go's JSON decoder maps both [] and null to a nil
	// slice, so a struct-level check would silently miss the bug.
	p := activeProduct()
	p.Nutrition = catalogue.Nutrition{ServingSize: "", Rows: nil}
	h, issuer := newAdminCatalogueAPI(t, &stubWriter{products: []catalogue.Product{p}})
	tok := issueAdminToken(t, issuer)

	rec := doAdminCatalogue(t, h, http.MethodGet, "/api/v1/admin/products/tablets-120", "", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"rows":[]`) {
		t.Errorf("body = %s, want nutrition.rows to serialise as []", rec.Body.String())
	}
}

/* -------------------------------- PUT /api/v1/admin/products/{id} */

func TestAdminUpdateProductReturns200(t *testing.T) {
	h, issuer := newAdminCatalogueAPI(t, &stubWriter{products: []catalogue.Product{activeProduct()}})
	tok := issueAdminToken(t, issuer)

	updated := `{
		"family": "tablets",
		"form": "Tablets",
		"name": "Updated Tablets 120",
		"mrp": 50000,
		"price": 40000,
		"stock": 80,
		"position": 2,
		"active": true,
		"rating": {"score": 4.9, "count": 400},
		"badges": [],
		"images": [],
		"nutrition": {}
	}`
	rec := doAdminCatalogue(t, h, http.MethodPut, "/api/v1/admin/products/tablets-120", updated, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var body struct {
		Data struct {
			Name string `json:"name"`
			MRP  int64  `json:"mrp"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.Name != "Updated Tablets 120" {
		t.Errorf("name = %q, want Updated Tablets 120", body.Data.Name)
	}
}

func TestAdminUpdateProductRejects422OnIDMismatch(t *testing.T) {
	h, issuer := newAdminCatalogueAPI(t, &stubWriter{products: []catalogue.Product{activeProduct()}})
	tok := issueAdminToken(t, issuer)

	// Body id does not match path id.
	body := `{
		"id": "different-id",
		"family": "tablets",
		"form": "Tablets",
		"name": "Tablets",
		"mrp": 47000,
		"price": 38000,
		"active": true,
		"rating": {"score": 0, "count": 0},
		"badges": [],
		"images": [],
		"nutrition": {}
	}`
	rec := doAdminCatalogue(t, h, http.MethodPut, "/api/v1/admin/products/tablets-120", body, tok)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422 for id mismatch", rec.Code)
	}
	if msg := catalogueAdminErrorMessage(t, rec); msg != "A product id cannot be changed." {
		t.Errorf("message = %q, want exact id-mismatch message", msg)
	}
}

func TestAdminUpdateProductAcceptsMatchingBodyID(t *testing.T) {
	// Body id matching the path id is allowed.
	h, issuer := newAdminCatalogueAPI(t, &stubWriter{products: []catalogue.Product{activeProduct()}})
	tok := issueAdminToken(t, issuer)

	body := `{
		"id": "tablets-120",
		"family": "tablets",
		"form": "Tablets",
		"name": "Updated",
		"mrp": 47000,
		"price": 38000,
		"active": true,
		"rating": {"score": 4.8, "count": 312},
		"badges": [],
		"images": [],
		"nutrition": {}
	}`
	rec := doAdminCatalogue(t, h, http.MethodPut, "/api/v1/admin/products/tablets-120", body, tok)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 when body id matches path id", rec.Code)
	}
}

func TestAdminUpdateProductReturns404ForUnknownID(t *testing.T) {
	h, issuer := newAdminCatalogueAPI(t, &stubWriter{products: []catalogue.Product{}})
	tok := issueAdminToken(t, issuer)

	body := `{
		"family": "tablets",
		"form": "Tablets",
		"name": "X",
		"mrp": 47000,
		"price": 38000,
		"active": true,
		"rating": {"score": 0, "count": 0},
		"badges": [],
		"images": [],
		"nutrition": {}
	}`
	rec := doAdminCatalogue(t, h, http.MethodPut, "/api/v1/admin/products/does-not-exist", body, tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

/* ----------------------------- DELETE /api/v1/admin/products/{id} */

func TestAdminRetireProductReturns200WithRetiredProduct(t *testing.T) {
	h, issuer := newAdminCatalogueAPI(t, &stubWriter{products: []catalogue.Product{activeProduct()}})
	tok := issueAdminToken(t, issuer)

	rec := doAdminCatalogue(t, h, http.MethodDelete, "/api/v1/admin/products/tablets-120", "", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var body struct {
		Data struct {
			ID     string `json:"id"`
			Active bool   `json:"active"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.ID != "tablets-120" {
		t.Errorf("id = %q, want tablets-120", body.Data.ID)
	}
	if body.Data.Active {
		t.Error("active = true after retire, want false")
	}
}

func TestAdminRetireAlreadyRetiredProductReturns200(t *testing.T) {
	retired := activeProduct()
	retired.Active = false
	h, issuer := newAdminCatalogueAPI(t, &stubWriter{products: []catalogue.Product{retired}})
	tok := issueAdminToken(t, issuer)

	rec := doAdminCatalogue(t, h, http.MethodDelete, "/api/v1/admin/products/tablets-120", "", tok)
	// Retiring an already-retired product is a no-op that still returns 200.
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for already-retired product", rec.Code)
	}
}

func TestAdminRetireProductReturns404ForUnknownID(t *testing.T) {
	h, issuer := newAdminCatalogueAPI(t, &stubWriter{products: []catalogue.Product{}})
	tok := issueAdminToken(t, issuer)

	rec := doAdminCatalogue(t, h, http.MethodDelete, "/api/v1/admin/products/does-not-exist", "", tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

/* ----------------------------------------- method / path fallbacks */

func TestAdminProductsWrongMethodReturns405(t *testing.T) {
	h, issuer := newAdminCatalogueAPI(t, &stubWriter{products: []catalogue.Product{activeProduct()}})
	tok := issueAdminToken(t, issuer)

	// PATCH is not a registered method on /admin/products.
	rec := doAdminCatalogue(t, h, http.MethodPatch, "/api/v1/admin/products", "", tok)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
	if code := catalogueAdminErrorCode(t, rec); code != "METHOD_NOT_ALLOWED" {
		t.Errorf("error.code = %q, want METHOD_NOT_ALLOWED", code)
	}
}

func TestAdminProductsUnknownPathReturns404(t *testing.T) {
	h, issuer := newAdminCatalogueAPI(t, &stubWriter{})
	tok := issueAdminToken(t, issuer)

	rec := doAdminCatalogue(t, h, http.MethodGet, "/api/v1/admin/products/a/b/c", "", tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

/* ----------------------------------------- shopper view excludes retired */

func TestShopperListExcludesRetiredProducts(t *testing.T) {
	// This test confirms that GET /products (shopper) still excludes retired
	// products even when the admin list includes them.
	active := activeProduct()
	retired := catalogue.Product{
		ID:     "tablets-60",
		Family: "tablets",
		Form:   catalogue.FormTablets,
		Name:   "Tablets 60",
		MRP:    30000,
		Price:  25000,
		Stock:  0,
		Active: false,
	}
	// Use the shopper-facing API by wiring a stubReader that reflects both products.
	reader := &stubReader{products: []catalogue.Product{active, retired}}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	h := server.New(server.Deps{
		Config:    config.Config{},
		Mongo:     stubPinger{},
		Catalogue: catalogue.NewHandler(catalogue.NewService(reader), logger),
		Logger:    logger,
		Version:   "test",
		Started:   time.Now(),
	})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/products", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Data struct {
			Products []struct {
				ID string `json:"id"`
			} `json:"products"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, p := range body.Data.Products {
		if p.ID == "tablets-60" {
			t.Error("retired product tablets-60 must not appear in the shopper list")
		}
	}
}
