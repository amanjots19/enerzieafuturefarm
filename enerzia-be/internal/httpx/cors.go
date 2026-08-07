package httpx

import "net/http"

const (
	corsAllowedMethods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
	corsAllowedHeaders = "Authorization, Content-Type"
)

// CORS returns middleware enforcing the cross-origin resource sharing policy.
// Only origins in allowedOrigins (exact string match) receive CORS response
// headers; the request origin is echoed back and a wildcard is never emitted.
// Credentials are not enabled — tokens travel in the Authorization header.
//
// Requests without an Origin header pass through untouched. Requests from a
// disallowed origin reach the handler without CORS headers so the browser
// blocks them; the server never returns 403. Preflight requests (OPTIONS +
// Access-Control-Request-Method) from an allowed origin are answered with 204
// and do not reach any downstream handler.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[o] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}
			// Vary must be set regardless of whether the origin is allowed so
			// shared caches do not serve one origin's CORS headers to another.
			w.Header().Set("Vary", "Origin")
			if !allowed[origin] {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.Header().Set("Access-Control-Allow-Methods", corsAllowedMethods)
				w.Header().Set("Access-Control-Allow-Headers", corsAllowedHeaders)
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
