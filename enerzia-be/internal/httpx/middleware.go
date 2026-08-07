package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

// RequestIDHeader carries the correlation id on both the request and response.
const RequestIDHeader = "X-Request-Id"

type ctxKey int

const requestIDKey ctxKey = iota

// RequestIDFrom returns the correlation id attached to ctx, or "" if the
// request did not pass through RequestID.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// newRequestID returns a random 16-character hex id. crypto/rand.Read never
// fails on the supported platforms, so there is no error branch to handle.
func newRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// RequestID reuses an inbound X-Request-Id when the caller supplied one and
// otherwise generates one, so a request can be traced across services. The id
// is echoed on the response and stored on the context.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(RequestIDHeader)
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set(RequestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

// statusRecorder captures the status and size for the access log, since
// http.ResponseWriter does not expose what was written.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusRecorder) WriteHeader(status int) {
	s.status = status
	s.ResponseWriter.WriteHeader(status)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

// Logging writes one structured line per request. It deliberately records only
// method, path, status, size and duration — never headers, body or query
// values, which can carry tokens and OTP codes.
func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}

			next.ServeHTTP(rec, r)

			if rec.status == 0 {
				rec.status = http.StatusOK
			}
			logger.InfoContext(r.Context(), "http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Int("bytes", rec.bytes),
				slog.Duration("duration", time.Since(start)),
				slog.String("request_id", RequestIDFrom(r.Context())),
			)
		})
	}
}

// Recover turns a panic in any downstream handler into a 500 in the standard
// envelope, logging the stack rather than returning it to the caller.
func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			path := r.URL.Path
			defer func() {
				if rec := recover(); rec != nil {
					reportPanic(ctx, logger, w, path, rec)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// reportPanic logs a recovered panic with its stack and answers with a generic
// 500. The panic value never reaches the client — it routinely contains
// internal detail.
func reportPanic(ctx context.Context, logger *slog.Logger, w http.ResponseWriter, path string, rec any) {
	logger.ErrorContext(ctx, "panic recovered",
		slog.Any("panic", rec),
		slog.String("path", path),
		slog.String("request_id", RequestIDFrom(ctx)),
		slog.String("stack", string(debug.Stack())),
	)
	WriteError(w, CodeInternal, "Something went wrong on our side.")
}
