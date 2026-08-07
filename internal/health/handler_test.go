package health_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/enerzia/enerzia-be/internal/health"
)

// stubPinger returns a fixed result and records that it was called.
type stubPinger struct {
	err    error
	called bool
}

func (s *stubPinger) Ping(context.Context) error {
	s.called = true
	return s.err
}

func decodeData(t *testing.T, rec *httptest.ResponseRecorder) health.Response {
	t.Helper()
	var body struct {
		Data health.Response `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v (%s)", err, rec.Body.String())
	}
	return body.Data
}

func serve(t *testing.T, p health.Pinger, started time.Time) (*httptest.ResponseRecorder, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	rec := httptest.NewRecorder()
	health.NewHandler(p, "0.1.0", started, logger).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	return rec, &buf
}

func TestHealthOKWhenMongoIsUp(t *testing.T) {
	pinger := &stubPinger{}
	rec, _ := serve(t, pinger, time.Now().Add(-90*time.Second))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	got := decodeData(t, rec)
	if got.Status != "ok" {
		t.Errorf("status = %q, want ok", got.Status)
	}
	if got.Mongo != "up" {
		t.Errorf("mongo = %q, want up", got.Mongo)
	}
	if got.Version != "0.1.0" {
		t.Errorf("version = %q, want 0.1.0", got.Version)
	}
	if got.UptimeSeconds < 89 || got.UptimeSeconds > 92 {
		t.Errorf("uptimeSeconds = %d, want about 90", got.UptimeSeconds)
	}
	if !pinger.called {
		t.Error("the dependency was never pinged")
	}
}

func TestHealthDegradedWhenMongoIsDown(t *testing.T) {
	rec, logs := serve(t,
		&stubPinger{err: errors.New("dial tcp cluster0.mongodb.net:27017: refused")},
		time.Now())

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	got := decodeData(t, rec)
	if got.Status != "degraded" {
		t.Errorf("status = %q, want degraded", got.Status)
	}
	if got.Mongo != "down" {
		t.Errorf("mongo = %q, want down", got.Mongo)
	}
	if !strings.Contains(logs.String(), "mongo unreachable") {
		t.Error("the failure must be logged")
	}
}

func TestHealthDoesNotLeakDriverErrorToClient(t *testing.T) {
	// Driver errors name hosts and sometimes users; they belong in the log.
	rec, logs := serve(t, &stubPinger{err: errors.New("auth failed for user appuser")}, time.Now())

	if strings.Contains(rec.Body.String(), "appuser") {
		t.Errorf("response leaks driver detail: %s", rec.Body.String())
	}
	if !strings.Contains(logs.String(), "appuser") {
		t.Error("the driver error should still reach the log")
	}
}

func TestHealthAlwaysWritesABody(t *testing.T) {
	for _, tt := range []struct {
		name   string
		pinger *stubPinger
	}{
		{"up", &stubPinger{}},
		{"down", &stubPinger{err: errors.New("nope")}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec, _ := serve(t, tt.pinger, time.Now())
			if rec.Body.Len() == 0 {
				t.Error("health must never answer with an empty body")
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("Content-Type = %q, want JSON", ct)
			}
		})
	}
}
