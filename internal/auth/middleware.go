package auth

import (
	"context"
	"net/http"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/enerzia/enerzia-be/internal/httpx"
)

type ctxKey int

const userIDKey ctxKey = iota

// bearerPrefix is the only authorization scheme accepted.
const bearerPrefix = "Bearer "

// UserIDFrom returns the signed-in user attached by Require. The bool is false
// on any request that did not pass through it.
func UserIDFrom(ctx context.Context) (bson.ObjectID, bool) {
	id, ok := ctx.Value(userIDKey).(bson.ObjectID)
	return id, ok
}

// withUserID returns a context carrying the signed-in user.
func withUserID(ctx context.Context, id bson.ObjectID) context.Context {
	return context.WithValue(ctx, userIDKey, id)
}

// TokenParser is the verification surface the middleware needs.
type TokenParser interface {
	ParseToken(token string) (Claims, error)
}

// Require rejects any request without a valid bearer token, and attaches the
// user id for handlers behind it.
//
// Every failure returns the same 401 and the same message. Distinguishing
// "malformed" from "expired" from "signed with the wrong key" would tell an
// attacker which part of a forged token to fix.
func Require(parser TokenParser) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				unauthorized(w)
				return
			}

			claims, err := parser.ParseToken(token)
			if err != nil {
				unauthorized(w)
				return
			}

			userID, err := claims.UserID()
			if err != nil {
				unauthorized(w)
				return
			}

			next.ServeHTTP(w, r.WithContext(withUserID(r.Context(), userID)))
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

func unauthorized(w http.ResponseWriter) {
	httpx.WriteError(w, httpx.CodeUnauthorized, "Please sign in to continue.")
}
