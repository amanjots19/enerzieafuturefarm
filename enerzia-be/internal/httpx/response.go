// Package httpx holds the shared HTTP plumbing: the response envelope defined
// in roadmap.md, the error-code vocabulary, and middleware.
package httpx

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// ErrorCode is the machine-readable classifier in an error response.
type ErrorCode string

// The error codes defined in roadmap.md. Each maps to exactly one status.
const (
	CodeValidation   ErrorCode = "VALIDATION_ERROR"
	CodeBadRequest   ErrorCode = "BAD_REQUEST"
	CodeUnauthorized ErrorCode = "UNAUTHORIZED"
	CodeNotFound     ErrorCode = "NOT_FOUND"
	CodeConflict     ErrorCode = "CONFLICT"
	CodeRateLimited  ErrorCode = "RATE_LIMITED"
	CodeInternal     ErrorCode = "INTERNAL"
	// CodeGatewayError is returned when an upstream payment gateway fails.
	CodeGatewayError ErrorCode = "GATEWAY_ERROR"
	// CodeMethodNotAllowed is returned for a known path with the wrong verb.
	CodeMethodNotAllowed ErrorCode = "METHOD_NOT_ALLOWED"
)

// statusFor maps each code to its HTTP status, keeping the two from drifting
// apart across handlers.
var statusFor = map[ErrorCode]int{
	CodeValidation:       http.StatusUnprocessableEntity,
	CodeBadRequest:       http.StatusBadRequest,
	CodeUnauthorized:     http.StatusUnauthorized,
	CodeNotFound:         http.StatusNotFound,
	CodeConflict:         http.StatusConflict,
	CodeRateLimited:      http.StatusTooManyRequests,
	CodeInternal:         http.StatusInternalServerError,
	CodeGatewayError:     http.StatusBadGateway,
	CodeMethodNotAllowed: http.StatusMethodNotAllowed,
}

// StatusFor returns the HTTP status for a code, defaulting to 500 for a code
// that has not been registered.
func StatusFor(code ErrorCode) int {
	if s, ok := statusFor[code]; ok {
		return s
	}
	return http.StatusInternalServerError
}

// FieldError names the request field that failed validation and the
// shopper-facing message explaining why.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ErrorBody is the value carried under the "error" key.
type ErrorBody struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Details any       `json:"details,omitempty"`
}

type dataEnvelope struct {
	Data any `json:"data"`
}

type errorEnvelope struct {
	Error ErrorBody `json:"error"`
}

// WriteJSON serialises v as the whole response body. Callers should normally
// use WriteData or WriteError so every response keeps the envelope shape.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// The status and part of the body may already be on the wire, so the
		// only useful action left is to record it.
		slog.Error("write json response", slog.Any("error", err))
	}
}

// WriteData sends a success envelope: {"data": ...}.
func WriteData(w http.ResponseWriter, status int, data any) {
	WriteJSON(w, status, dataEnvelope{Data: data})
}

// WriteError sends an error envelope with the status registered for code.
func WriteError(w http.ResponseWriter, code ErrorCode, message string) {
	WriteErrorDetails(w, code, message, nil)
}

// WriteErrorDetails sends an error envelope carrying structured details, such
// as the ordered field failures described in roadmap.md.
func WriteErrorDetails(w http.ResponseWriter, code ErrorCode, message string, details any) {
	WriteJSON(w, StatusFor(code), errorEnvelope{Error: ErrorBody{
		Code:    code,
		Message: message,
		Details: details,
	}})
}

// WriteFieldErrors sends a validation failure. The frontend shows only the
// first entry, so order matters — see product.md §3.4.
func WriteFieldErrors(w http.ResponseWriter, fields []FieldError) {
	message := "The request could not be processed."
	if len(fields) > 0 {
		message = fields[0].Message
	}
	WriteErrorDetails(w, CodeValidation, message, map[string]any{"fields": fields})
}

// NotFoundHandler answers unknown routes in the standard envelope rather than
// gorilla/mux's plain-text default.
func NotFoundHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		WriteError(w, CodeNotFound, "That endpoint does not exist.")
	})
}

// MethodNotAllowedHandler answers a known path used with the wrong verb.
func MethodNotAllowedHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		WriteError(w, CodeMethodNotAllowed, "That method is not allowed on this endpoint.")
	})
}
