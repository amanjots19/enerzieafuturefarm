package httpx_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/enerzia/enerzia-be/internal/httpx"
)

// captureLogger returns a logger writing JSON into buf, so tests can assert on
// what was — and was not — logged.
func captureLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})), &buf
}

func TestRequestIDGeneratesWhenAbsent(t *testing.T) {
	var seen string
	h := httpx.RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = httpx.RequestIDFrom(r.Context())
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if seen == "" {
		t.Fatal("no request id on the context")
	}
	if got := rec.Header().Get(httpx.RequestIDHeader); got != seen {
		t.Errorf("response header %q = %q, want %q", httpx.RequestIDHeader, got, seen)
	}
	if len(seen) != 16 {
		t.Errorf("generated id %q has length %d, want 16 hex chars", seen, len(seen))
	}
}

func TestRequestIDGeneratesDistinctIDs(t *testing.T) {
	var ids []string
	h := httpx.RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		ids = append(ids, httpx.RequestIDFrom(r.Context()))
	}))
	for range 2 {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	}
	if ids[0] == ids[1] {
		t.Errorf("two requests share the id %q", ids[0])
	}
}

func TestRequestIDReusesInboundHeader(t *testing.T) {
	var seen string
	h := httpx.RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = httpx.RequestIDFrom(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(httpx.RequestIDHeader, "trace-from-caller")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if seen != "trace-from-caller" {
		t.Errorf("request id = %q, want the inbound value to be reused", seen)
	}
	if rec.Header().Get(httpx.RequestIDHeader) != "trace-from-caller" {
		t.Error("inbound id must also be echoed on the response")
	}
}

func TestRequestIDFromWithoutMiddleware(t *testing.T) {
	if got := httpx.RequestIDFrom(t.Context()); got != "" {
		t.Errorf("RequestIDFrom(bare ctx) = %q, want empty", got)
	}
}

func TestLoggingRecordsRequestFacts(t *testing.T) {
	logger, buf := captureLogger()
	h := httpx.RequestID(httpx.Logging(logger)(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTeapot)
			_, _ = w.Write([]byte("hi"))
		})))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/cart", nil))

	var line map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &line); err != nil {
		t.Fatalf("log line is not JSON: %v (%s)", err, buf.String())
	}
	if line["method"] != http.MethodPost {
		t.Errorf("method = %v, want POST", line["method"])
	}
	if line["path"] != "/api/v1/cart" {
		t.Errorf("path = %v", line["path"])
	}
	if line["status"] != float64(http.StatusTeapot) {
		t.Errorf("status = %v, want 418", line["status"])
	}
	if line["bytes"] != float64(2) {
		t.Errorf("bytes = %v, want 2", line["bytes"])
	}
	if line["request_id"] == "" {
		t.Error("request_id must be logged")
	}
}

func TestLoggingDefaultsStatusWhenHandlerNeverSetsOne(t *testing.T) {
	logger, buf := captureLogger()
	// Handler writes nothing at all: net/http would send 200.
	h := httpx.Logging(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	var line map[string]any
	_ = json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &line)
	if line["status"] != float64(http.StatusOK) {
		t.Errorf("status = %v, want 200 when the handler writes nothing", line["status"])
	}
}

func TestLoggingDefaultsStatusOnBareWrite(t *testing.T) {
	logger, buf := captureLogger()
	// Write without WriteHeader implies 200.
	h := httpx.Logging(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("body"))
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	var line map[string]any
	_ = json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &line)
	if line["status"] != float64(http.StatusOK) {
		t.Errorf("status = %v, want 200", line["status"])
	}
	if line["bytes"] != float64(4) {
		t.Errorf("bytes = %v, want 4", line["bytes"])
	}
}

func TestLoggingDoesNotRecordQueryOrHeaders(t *testing.T) {
	// Query strings and headers can carry tokens and OTP codes.
	logger, buf := captureLogger()
	h := httpx.Logging(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/products?token=super-secret", nil)
	req.Header.Set("Authorization", "Bearer leak-me")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if out := buf.String(); strings.Contains(out, "super-secret") || strings.Contains(out, "leak-me") {
		t.Errorf("access log leaked sensitive data: %s", out)
	}
}

func TestRecoverConvertsPanicToEnvelope(t *testing.T) {
	logger, buf := captureLogger()
	h := httpx.Recover(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom in a handler")
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/cart", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	errObj := decode(t, rec)["error"].(map[string]any)
	if errObj["code"] != string(httpx.CodeInternal) {
		t.Errorf("error.code = %v, want INTERNAL", errObj["code"])
	}
	if msg, _ := errObj["message"].(string); strings.Contains(msg, "boom") {
		t.Errorf("panic value leaked to the client: %q", msg)
	}
	if !strings.Contains(buf.String(), "boom in a handler") {
		t.Error("panic value must still reach the log")
	}
	if !strings.Contains(buf.String(), "stack") {
		t.Error("stack trace must be logged")
	}
}

func TestRecoverLeavesNormalResponsesAlone(t *testing.T) {
	logger, buf := captureLogger()
	h := httpx.Recover(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteData(w, http.StatusOK, "fine")
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if strings.Contains(buf.String(), "panic recovered") {
		t.Error("nothing should be logged as a panic")
	}
}
