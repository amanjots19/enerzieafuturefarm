package admin

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"github.com/enerzia/enerzia-be/internal/cloudinary"
	"github.com/enerzia/enerzia-be/internal/httpx"
)

// Handler serves the admin endpoints defined in roadmap.md §Admin.
type Handler struct {
	service *Service
	logger  *slog.Logger
	signer  cloudinary.Signer
	now     func() time.Time
}

// NewHandler builds the admin HTTP handler. Cloudinary is disabled by default;
// call WithCloudinary to enable the signed-upload endpoint.
func NewHandler(service *Service, logger *slog.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  logger,
		signer:  cloudinary.Unconfigured{},
		now:     time.Now,
	}
}

// WithCloudinary returns the handler with Cloudinary upload signing enabled.
// now is the clock used to generate the upload timestamp; pass time.Now in
// production and a fixed clock in tests.
func (h *Handler) WithCloudinary(signer cloudinary.Signer, now func() time.Time) *Handler {
	h.signer = signer
	h.now = now
	return h
}

// Register mounts the admin routes on the /api/v1 subrouter.
//
// Routes are grouped under an /admin sub-prefix. The secure sub-subrouter
// carries the Require middleware so only /admin/me is gated; /admin/login is
// intentionally outside it.
func (h *Handler) Register(r *mux.Router) {
	sub := r.PathPrefix("/admin").Subrouter()
	sub.HandleFunc("/login", h.login).Methods(http.MethodPost)

	secure := sub.NewRoute().Subrouter()
	secure.Use(Require(h.service))
	secure.HandleFunc("/me", h.me).Methods(http.MethodGet)
	secure.HandleFunc("/uploads/signature", h.uploadSignature).Methods(http.MethodPost)
}

/* ---------------------------------------------------------------- wire DTOs */

type loginBody struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
	Email     string    `json:"email"`
}

type meResponse struct {
	Email string `json:"email"`
}

/* ----------------------------------------------------------------- handlers */

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var body loginBody
	if !decodeAdminJSON(w, r, &body) {
		return
	}

	// Validate in field order: email first, then password.
	if strings.TrimSpace(body.Email) == "" {
		httpx.WriteFieldErrors(w, []httpx.FieldError{
			{Field: "email", Message: "Email is required"},
		})
		return
	}
	if strings.TrimSpace(body.Password) == "" {
		httpx.WriteFieldErrors(w, []httpx.FieldError{
			{Field: "password", Message: "Password is required"},
		})
		return
	}

	ip := remoteIP(r)
	tok, expiresAt, err := h.service.Login(r.Context(), ip, body.Email, body.Password)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotConfigured):
			// Log the misconfiguration server-side; the client gets the same
			// 401 as a wrong password so callers cannot distinguish the states.
			h.logger.ErrorContext(r.Context(), "admin: login attempted but admin credentials not configured",
				slog.String("request_id", httpx.RequestIDFrom(r.Context())))
			httpx.WriteError(w, httpx.CodeUnauthorized, "Email or password is incorrect.")
		case errors.Is(err, ErrRateLimited):
			httpx.WriteError(w, httpx.CodeRateLimited, "Too many sign-in attempts. Try again later.")
		case errors.Is(err, ErrInvalidCredentials):
			httpx.WriteError(w, httpx.CodeUnauthorized, "Email or password is incorrect.")
		default:
			h.fail(r, w, "login", err)
		}
		return
	}

	httpx.WriteData(w, http.StatusOK, loginResponse{
		Token:     tok,
		ExpiresAt: expiresAt.UTC(),
		// Return the canonical configured email, not body.Email, so the
		// response is consistent with what GET /admin/me returns.
		Email: h.service.Email(),
	})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	email, ok := EmailFrom(r.Context())
	if !ok {
		// Unreachable behind Require; guarded so a routing mistake cannot
		// silently serve an anonymous request.
		httpx.WriteError(w, httpx.CodeUnauthorized, "Please sign in to continue.")
		return
	}
	httpx.WriteData(w, http.StatusOK, meResponse{Email: email})
}

func (h *Handler) uploadSignature(w http.ResponseWriter, r *http.Request) {
	sig, err := h.signer.SignUpload(h.now().Unix())
	if err != nil {
		if errors.Is(err, cloudinary.ErrNotConfigured) {
			h.logger.ErrorContext(r.Context(), "admin: cloudinary not configured",
				slog.String("request_id", httpx.RequestIDFrom(r.Context())))
			httpx.WriteError(w, httpx.CodeInternal, "Something went wrong on our side.")
			return
		}
		h.fail(r, w, "upload signature", err)
		return
	}
	httpx.WriteData(w, http.StatusOK, sig)
}

/* ------------------------------------------------------------------ helpers */

// decodeAdminJSON reads a JSON body, answering 400 and returning false if it
// cannot. Unknown fields are rejected so a typo in a client payload is not
// silently ignored. The body is never logged — it may contain a password.
func decodeAdminJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	const maxBodyBytes = 4 << 10
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		httpx.WriteError(w, httpx.CodeBadRequest, "The request body could not be read.")
		return false
	}
	return true
}

// remoteIP extracts the host portion of r.RemoteAddr. X-Forwarded-For is
// deliberately NOT used — see Limiter for the full rationale.
func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// fail logs the internal error and returns a generic 500.
func (h *Handler) fail(r *http.Request, w http.ResponseWriter, op string, err error) {
	h.logger.ErrorContext(r.Context(), "admin: "+op,
		slog.Any("error", err),
		slog.String("request_id", httpx.RequestIDFrom(r.Context())),
	)
	httpx.WriteError(w, httpx.CodeInternal, "Something went wrong on our side.")
}
