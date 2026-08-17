package admin

import (
	"context"
	"net/http"
	"strings"

	"github.com/enerzia/enerzia-be/internal/httpx"
)

// contextKey is an unexported type used as a context key to avoid collisions
// with other packages that store values in request contexts.
type contextKey int

const emailKey contextKey = iota

// bearerPrefix is the only authorization scheme accepted.
const bearerPrefix = "Bearer "

// TokenParser is the subset of Service that middleware needs to verify tokens.
// Accepting an interface keeps the middleware decoupled from the full Service
// type and makes it easy to substitute a test double.
type TokenParser interface {
	ParseToken(token string) (Claims, error)
}

// EmailFrom retrieves the authenticated admin email from ctx. The second
// return value is false when no email is present (i.e. Require middleware did
// not run or the token was invalid).
func EmailFrom(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(emailKey).(string)
	return v, ok && v != ""
}

// Require is HTTP middleware that validates the admin bearer token and stores
// the email in the request context. Requests without a valid token receive 401.
func Require(parser TokenParser) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, ok := bearerToken(r)
			if !ok {
				httpx.WriteError(w, httpx.CodeUnauthorized, "Please sign in to continue.")
				return
			}
			claims, err := parser.ParseToken(raw)
			if err != nil {
				httpx.WriteError(w, httpx.CodeUnauthorized, "Please sign in to continue.")
				return
			}
			ctx := context.WithValue(r.Context(), emailKey, claims.Subject)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// bearerToken extracts the credential from the Authorization header. The
// scheme is matched case-insensitively, as RFC 7235 requires.
func bearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	if len(header) < len(bearerPrefix) {
		return "", false
	}
	if !strings.EqualFold(header[:len(bearerPrefix)], bearerPrefix) {
		return "", false
	}
	token := strings.TrimSpace(header[len(bearerPrefix):])
	return token, token != ""
}
