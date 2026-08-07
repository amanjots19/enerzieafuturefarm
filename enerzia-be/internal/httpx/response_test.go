package httpx_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/enerzia/enerzia-be/internal/httpx"
)

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response body is not JSON: %v (%s)", err, rec.Body.String())
	}
	return body
}

func TestStatusFor(t *testing.T) {
	tests := []struct {
		code httpx.ErrorCode
		want int
	}{
		{httpx.CodeValidation, http.StatusUnprocessableEntity},
		{httpx.CodeBadRequest, http.StatusBadRequest},
		{httpx.CodeUnauthorized, http.StatusUnauthorized},
		{httpx.CodeNotFound, http.StatusNotFound},
		{httpx.CodeConflict, http.StatusConflict},
		{httpx.CodeRateLimited, http.StatusTooManyRequests},
		{httpx.CodeInternal, http.StatusInternalServerError},
		{httpx.CodeMethodNotAllowed, http.StatusMethodNotAllowed},
		{httpx.ErrorCode("SOMETHING_NEW"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			if got := httpx.StatusFor(tt.code); got != tt.want {
				t.Errorf("StatusFor(%q) = %d, want %d", tt.code, got, tt.want)
			}
		})
	}
}

func TestWriteDataWrapsInEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	httpx.WriteData(rec, http.StatusCreated, map[string]string{"orderId": "EFF-123456"})

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	body := decode(t, rec)
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("body has no data object: %v", body)
	}
	if data["orderId"] != "EFF-123456" {
		t.Errorf("data.orderId = %v, want EFF-123456", data["orderId"])
	}
	if _, hasErr := body["error"]; hasErr {
		t.Error("success envelope must not carry an error key")
	}
}

func TestWriteErrorUsesRegisteredStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	httpx.WriteError(rec, httpx.CodeConflict, "Your cart is empty.")

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
	errObj, ok := decode(t, rec)["error"].(map[string]any)
	if !ok {
		t.Fatal("body has no error object")
	}
	if errObj["code"] != string(httpx.CodeConflict) {
		t.Errorf("error.code = %v, want %v", errObj["code"], httpx.CodeConflict)
	}
	if errObj["message"] != "Your cart is empty." {
		t.Errorf("error.message = %v", errObj["message"])
	}
	if _, has := errObj["details"]; has {
		t.Error("details must be omitted when nil")
	}
}

func TestWriteErrorDetailsIncludesDetails(t *testing.T) {
	rec := httptest.NewRecorder()
	httpx.WriteErrorDetails(rec, httpx.CodeRateLimited, "Too many attempts.",
		map[string]any{"retryAfterSeconds": 30})

	errObj := decode(t, rec)["error"].(map[string]any)
	details, ok := errObj["details"].(map[string]any)
	if !ok {
		t.Fatalf("details missing: %v", errObj)
	}
	if details["retryAfterSeconds"] != float64(30) {
		t.Errorf("details.retryAfterSeconds = %v, want 30", details["retryAfterSeconds"])
	}
}

func TestWriteFieldErrorsUsesFirstMessage(t *testing.T) {
	// The UI shows only the first failure, so the top-level message must be
	// the first field's message and the order must be preserved.
	rec := httptest.NewRecorder()
	httpx.WriteFieldErrors(rec, []httpx.FieldError{
		{Field: "name", Message: "Please enter the name for delivery."},
		{Field: "pin", Message: "PIN code must be 6 digits."},
	})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", rec.Code)
	}
	errObj := decode(t, rec)["error"].(map[string]any)
	if errObj["message"] != "Please enter the name for delivery." {
		t.Errorf("message = %v, want the first field's message", errObj["message"])
	}
	fields := errObj["details"].(map[string]any)["fields"].([]any)
	if len(fields) != 2 {
		t.Fatalf("fields length = %d, want 2", len(fields))
	}
	if fields[0].(map[string]any)["field"] != "name" {
		t.Error("field order must be preserved")
	}
}

func TestWriteFieldErrorsWithNoFields(t *testing.T) {
	rec := httptest.NewRecorder()
	httpx.WriteFieldErrors(rec, nil)

	errObj := decode(t, rec)["error"].(map[string]any)
	if errObj["message"] != "The request could not be processed." {
		t.Errorf("message = %v, want the generic fallback", errObj["message"])
	}
}

func TestWriteJSONLogsButDoesNotPanicOnUnserialisableValue(t *testing.T) {
	// A channel cannot be marshalled; the helper must not panic mid-response.
	rec := httptest.NewRecorder()
	httpx.WriteJSON(rec, http.StatusOK, make(chan int))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (header is already committed)", rec.Code)
	}
}

func TestNotFoundHandler(t *testing.T) {
	rec := httptest.NewRecorder()
	httpx.NotFoundHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	if decode(t, rec)["error"].(map[string]any)["code"] != string(httpx.CodeNotFound) {
		t.Error("unknown routes must use the standard envelope")
	}
}

func TestMethodNotAllowedHandler(t *testing.T) {
	rec := httptest.NewRecorder()
	httpx.MethodNotAllowedHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/health", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
	if decode(t, rec)["error"].(map[string]any)["code"] != string(httpx.CodeMethodNotAllowed) {
		t.Error("405 must use the standard envelope")
	}
}
