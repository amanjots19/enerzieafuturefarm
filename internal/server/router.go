// Package server wires handlers and middleware into the HTTP router.
package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"github.com/enerzia/enerzia-be/internal/auth"
	"github.com/enerzia/enerzia-be/internal/cart"
	"github.com/enerzia/enerzia-be/internal/catalogue"
	"github.com/enerzia/enerzia-be/internal/config"
	"github.com/enerzia/enerzia-be/internal/health"
	"github.com/enerzia/enerzia-be/internal/httpx"
	"github.com/enerzia/enerzia-be/internal/order"
)

// APIPrefix is the version prefix every business endpoint sits behind. Health
// deliberately sits outside it.
const APIPrefix = "/api/v1"

// Deps are the collaborators the router needs. Everything is injected so tests
// can substitute fakes; there is no package-level state.
type Deps struct {
	Config    config.Config
	Mongo     health.Pinger
	Catalogue *catalogue.Handler
	Auth      *auth.Handler
	Cart      *cart.Handler
	Order     *order.Handler
	Logger    *slog.Logger
	Version   string
	Started   time.Time
}

// New builds the fully wired HTTP handler.
//
// Middleware order matters: RequestID runs first so the id exists for
// everything after it, Recover next so a panic in the logging wrapper or any
// handler still yields an enveloped 500, and Logging last so it records the
// status that Recover may have set.
func New(deps Deps) http.Handler {
	r := mux.NewRouter()

	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(deps.Logger))
	r.Use(httpx.Logging(deps.Logger))

	notFound := withMiddleware(deps, methodAware(r, httpx.NotFoundHandler(), httpx.MethodNotAllowedHandler()))
	methodNotAllowed := withMiddleware(deps, httpx.MethodNotAllowedHandler())

	r.NotFoundHandler = notFound
	r.MethodNotAllowedHandler = methodNotAllowed

	r.Handle("/health",
		health.NewHandler(deps.Mongo, deps.Version, deps.Started, deps.Logger),
	).Methods(http.MethodGet)

	// Business endpoints are registered on this subrouter as the roadmap
	// phases land.
	api := r.PathPrefix(APIPrefix).Subrouter()
	// A subrouter inherits neither handler.
	api.NotFoundHandler = notFound
	api.MethodNotAllowedHandler = methodNotAllowed

	if deps.Catalogue != nil {
		deps.Catalogue.Register(api)
	}
	if deps.Auth != nil {
		deps.Auth.Register(api)
	}
	if deps.Cart != nil {
		deps.Cart.Register(api)
	}
	if deps.Order != nil {
		deps.Order.Register(api)
		// Webhook sits outside /api/v1 and outside auth — register on root.
		deps.Order.RegisterWebhook(r)
	}

	return r
}

// withMiddleware applies the same chain to the router-level fallback handlers,
// which gorilla/mux does not route through r.Use.
func withMiddleware(deps Deps, h http.Handler) http.Handler {
	return httpx.RequestID(httpx.Recover(deps.Logger)(httpx.Logging(deps.Logger)(h)))
}

// probeMethods are the verbs checked when deciding whether an unmatched
// request is really a wrong-method request.
var probeMethods = []string{
	http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
	http.MethodPatch, http.MethodDelete, http.MethodOptions,
}

// methodAware answers 405 instead of 404 when the path exists under a
// different verb.
//
// gorilla/mux records a method mismatch on the route that matched by path, but
// clears it as soon as a *later* route fails on its own path matcher — so only
// the last-registered route ever produces a 405. Rather than depend on
// registration order, the signal is recovered here by re-matching the path
// under each other verb.
func methodAware(router *mux.Router, notFound, methodNotAllowed http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, method := range probeMethods {
			if method == r.Method {
				continue
			}
			probe := r.Clone(r.Context())
			probe.Method = method

			var match mux.RouteMatch
			// MatchErr nil means a real route matched, not a fallback.
			if router.Match(probe, &match) && match.MatchErr == nil {
				methodNotAllowed.ServeHTTP(w, r)
				return
			}
		}
		notFound.ServeHTTP(w, r)
	})
}
