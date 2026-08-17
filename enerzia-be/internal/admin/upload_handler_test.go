package admin_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/enerzia/enerzia-be/internal/admin"
	"github.com/enerzia/enerzia-be/internal/cloudinary"
	"github.com/enerzia/enerzia-be/internal/config"
	"github.com/enerzia/enerzia-be/internal/server"
)

// stubCloudinarySigner is an in-memory Signer for upload signature tests.
// It copies the pre-built Signature and stamps the live timestamp so tests can
// assert the timestamp field matches what the handler injected.
type stubCloudinarySigner struct {
	base cloudinary.Signature // fields other than Timestamp
	err  error
}

func (s stubCloudinarySigner) SignUpload(ts int64) (cloudinary.Signature, error) {
	if s.err != nil {
		return cloudinary.Signature{}, s.err
	}
	sig := s.base
	sig.Timestamp = ts
	return sig, nil
}

// newAdminAPIWithSigner builds the real router with the given Cloudinary signer
// and clock wired into the admin handler.
func newAdminAPIWithSigner(t *testing.T, signer cloudinary.Signer, now func() time.Time) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	tokens := admin.NewTokenIssuer(adminSecret, admin.TokenTTL)
	svc := admin.NewService(admin.ServiceConfig{
		Email:        testAdminEmail,
		PasswordHash: testPasswordHash,
		Tokens:       tokens,
		Limiter:      admin.NewLimiter(),
	})
	h := admin.NewHandler(svc, logger).WithCloudinary(signer, now)
	return server.New(server.Deps{
		Config:  config.Config{},
		Mongo:   stubPinger{},
		Admin:   h,
		Logger:  logger,
		Version: "test",
		Started: time.Now(),
	})
}

/* ------------------------------------------------ authentication guard tests */

func TestUploadSignatureWithoutTokenIs401(t *testing.T) {
	h := newAdminAPIWithSigner(t, stubCloudinarySigner{}, time.Now)
	rec := doAdmin(t, h, http.MethodPost, "/api/v1/admin/uploads/signature", "", "10.0.0.1:1234", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (no token); body: %s", rec.Code, rec.Body)
	}
	if got := adminErrorCode(t, rec); got != "UNAUTHORIZED" {
		t.Errorf("error code = %q, want UNAUTHORIZED", got)
	}
}

func TestUploadSignatureWithShopperTokenIs401(t *testing.T) {
	h := newAdminAPIWithSigner(t, stubCloudinarySigner{}, time.Now)

	// A shopper token has issuer "enerzia-api", not "enerzia-admin".
	shopperToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   "000000000000000000000001",
		"phone": "9876543210",
		"iss":   "enerzia-api",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}).SignedString(adminSecret)
	if err != nil {
		t.Fatalf("build shopper token: %v", err)
	}

	rec := doAdmin(t, h, http.MethodPost, "/api/v1/admin/uploads/signature", "", "10.0.0.1:1234", shopperToken)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (shopper token); body: %s", rec.Code, rec.Body)
	}
}

/* ----------------------------------------------- success response shape */

func TestUploadSignatureReturnsExpectedFields(t *testing.T) {
	signer := stubCloudinarySigner{
		base: cloudinary.Signature{
			CloudName: "testcloud",
			APIKey:    "testkey123",
			Folder:    "enerzia/products",
			Signature: "abcdef1234567890",
		},
	}
	fixedTime := time.Unix(1700000000, 0)
	h := newAdminAPIWithSigner(t, signer, func() time.Time { return fixedTime })
	token := loginAndGetToken(t, h)

	rec := doAdmin(t, h, http.MethodPost, "/api/v1/admin/uploads/signature", "", "10.0.0.1:1234", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}

	var resp struct {
		Data struct {
			CloudName string `json:"cloudName"`
			APIKey    string `json:"apiKey"`
			Timestamp int64  `json:"timestamp"`
			Folder    string `json:"folder"`
			Signature string `json:"signature"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response is not valid JSON: %v; body: %s", err, rec.Body)
	}

	if resp.Data.CloudName != "testcloud" {
		t.Errorf("cloudName = %q, want testcloud", resp.Data.CloudName)
	}
	if resp.Data.APIKey != "testkey123" {
		t.Errorf("apiKey = %q, want testkey123", resp.Data.APIKey)
	}
	if resp.Data.Timestamp != fixedTime.Unix() {
		t.Errorf("timestamp = %d, want %d (from injected clock)", resp.Data.Timestamp, fixedTime.Unix())
	}
	if resp.Data.Folder != "enerzia/products" {
		t.Errorf("folder = %q, want enerzia/products", resp.Data.Folder)
	}
	if resp.Data.Signature == "" {
		t.Error("signature is empty")
	}
}

func TestUploadSignatureDoesNotLeakAPISecret(t *testing.T) {
	const apiSecret = "the-cloudinary-api-secret-must-not-appear-in-response"
	// Use a real Client so the secret flows through the real signing path.
	client := cloudinary.NewClient("mycloud", "mykey", apiSecret, "enerzia/products")
	h := newAdminAPIWithSigner(t, client, func() time.Time { return time.Unix(1700000000, 0) })
	token := loginAndGetToken(t, h)

	rec := doAdmin(t, h, http.MethodPost, "/api/v1/admin/uploads/signature", "", "10.0.0.1:1234", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), apiSecret) {
		t.Errorf("response body contains the API secret: %s", rec.Body)
	}
}

/* ------------------------------------------------ error paths */

func TestUploadSignatureUnconfiguredIs500(t *testing.T) {
	h := newAdminAPIWithSigner(t, cloudinary.Unconfigured{}, time.Now)
	token := loginAndGetToken(t, h)

	rec := doAdmin(t, h, http.MethodPost, "/api/v1/admin/uploads/signature", "", "10.0.0.1:1234", token)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (Unconfigured); body: %s", rec.Code, rec.Body)
	}
	if got := adminErrorCode(t, rec); got != "INTERNAL" {
		t.Errorf("error code = %q, want INTERNAL", got)
	}
}

func TestUploadSignatureWrongMethodIs405(t *testing.T) {
	h := newAdminAPIWithSigner(t, stubCloudinarySigner{}, time.Now)
	token := loginAndGetToken(t, h)

	rec := doAdmin(t, h, http.MethodGet, "/api/v1/admin/uploads/signature", "", "10.0.0.1:1234", token)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405; body: %s", rec.Code, rec.Body)
	}
	if got := adminErrorCode(t, rec); got != "METHOD_NOT_ALLOWED" {
		t.Errorf("error code = %q, want METHOD_NOT_ALLOWED", got)
	}
}

/* ------------------------------------------------ clock injection */

func TestUploadSignatureDifferentTimesProduceDifferentSignatures(t *testing.T) {
	// Use a real Client so the timestamp actually affects the computed signature.
	client := cloudinary.NewClient("mycloud", "mykey", "mysecret", "enerzia/products")

	t1 := time.Unix(1700000000, 0)
	t2 := time.Unix(1700000001, 0)

	h1 := newAdminAPIWithSigner(t, client, func() time.Time { return t1 })
	h2 := newAdminAPIWithSigner(t, client, func() time.Time { return t2 })

	tok1 := loginAndGetToken(t, h1)
	tok2 := loginAndGetToken(t, h2)

	rec1 := doAdmin(t, h1, http.MethodPost, "/api/v1/admin/uploads/signature", "", "10.0.0.1:1234", tok1)
	rec2 := doAdmin(t, h2, http.MethodPost, "/api/v1/admin/uploads/signature", "", "10.0.0.1:1234", tok2)

	if rec1.Code != http.StatusOK || rec2.Code != http.StatusOK {
		t.Fatalf("status = %d / %d, want both 200", rec1.Code, rec2.Code)
	}

	var r1, r2 struct {
		Data struct {
			Signature string `json:"signature"`
			Timestamp int64  `json:"timestamp"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec1.Body.Bytes(), &r1); err != nil {
		t.Fatalf("parse r1: %v", err)
	}
	if err := json.Unmarshal(rec2.Body.Bytes(), &r2); err != nil {
		t.Fatalf("parse r2: %v", err)
	}

	if r1.Data.Signature == r2.Data.Signature {
		t.Error("different timestamps produced identical signatures; clock injection is broken")
	}
	if r1.Data.Timestamp == r2.Data.Timestamp {
		t.Error("timestamps in response are identical; clock injection is broken")
	}
}
