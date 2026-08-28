package auth

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/enerzia/enerzia-be/internal/httpx"
	"github.com/enerzia/enerzia-be/internal/msg91"
)

// Handler serves the auth endpoints defined in roadmap.md §Auth.
type Handler struct {
	service *Service
	logger  *slog.Logger
}

// NewHandler builds the auth HTTP handler.
func NewHandler(service *Service, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

// Register mounts the auth routes on the /api/v1 subrouter.
func (h *Handler) Register(r *mux.Router) {
	r.HandleFunc("/auth/session", h.session).Methods(http.MethodPost)
	r.HandleFunc("/auth/otp/request", h.requestCode).Methods(http.MethodPost)
	r.HandleFunc("/auth/otp/verify", h.verifyCode).Methods(http.MethodPost)

	me := r.PathPrefix("/auth/me").Subrouter()
	me.Use(Require(h.service))
	me.HandleFunc("", h.me).Methods(http.MethodGet)

	// The delivery address lives on the user document, so it is served here
	// rather than in its own package (schema.md §users).
	addr := r.PathPrefix("/me/addresses").Subrouter()
	addr.Use(Require(h.service))
	addr.HandleFunc("", h.listAddresses).Methods(http.MethodGet)
	addr.HandleFunc("", h.addAddress).Methods(http.MethodPost)
	addr.HandleFunc("/{addressId}", h.updateAddress).Methods(http.MethodPut)
	addr.HandleFunc("/{addressId}", h.deleteAddress).Methods(http.MethodDelete)
}

/* ---------------------------------------------------------------- wire DTOs */

type requestCodeBody struct {
	Phone string `json:"phone"`
}

type requestCodeResponse struct {
	ExpiresInSeconds   int    `json:"expiresInSeconds"`
	ResendAfterSeconds int    `json:"resendAfterSeconds"`
	DevCode            string `json:"devCode,omitempty"`
}

type verifyCodeBody struct {
	Phone string `json:"phone"`
	Code  string `json:"code"`
}

type userDTO struct {
	ID        string    `json:"id"`
	Phone     string    `json:"phone"`
	CreatedAt time.Time `json:"createdAt"`
}

type verifyCodeResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
	User      userDTO   `json:"user"`
}

type sessionBody struct {
	AccessToken string `json:"accessToken"`
}

type meResponse struct {
	User userDTO `json:"user"`
}

// addressListResponse carries an empty array rather than a 404 when none is
// saved — a blank address book is a state, not an error.
type addressListResponse struct {
	Addresses []Address `json:"addresses"`
}

type addressResponse struct {
	Address Address `json:"address"`
}

func toUserDTO(u User) userDTO {
	return userDTO{ID: u.ID.Hex(), Phone: u.Phone, CreatedAt: u.CreatedAt}
}

/* ----------------------------------------------------------------- handlers */

func (h *Handler) session(w http.ResponseWriter, r *http.Request) {
	var body sessionBody
	if !decodeJSON(w, r, &body) {
		return
	}

	if strings.TrimSpace(body.AccessToken) == "" {
		httpx.WriteError(w, httpx.CodeValidation, "Please sign in again.")
		return
	}

	result, err := h.service.CreateSession(r.Context(), body.AccessToken)
	if err != nil {
		switch {
		case errors.Is(err, ErrSessionRejected):
			// Log MSG91's diagnostic detail when available. The code and message
			// are diagnostic strings from MSG91, not secrets. Token length (not
			// the value) helps identify truncation as a cause.
			var ve *msg91.VerificationError
			var pe *PhoneRejectedError
			switch {
			case errors.As(err, &ve):
				h.logger.WarnContext(r.Context(), "auth: session token rejected by MSG91",
					slog.String("reason", "msg91_rejected"),
					slog.String("msg91_type", ve.Type),
					slog.String("msg91_code", ve.Code),
					slog.String("msg91_message", ve.Message),
					slog.Int("token_length", len(body.AccessToken)),
					slog.String("request_id", httpx.RequestIDFrom(r.Context())))
			case errors.As(err, &pe):
				// MSG91 was satisfied and WE refused the identifier. Logged
				// distinctly because these two used to be indistinguishable in
				// the journal, which is what made international sign-in failing
				// so hard to pin down. The shape is safe to log; the number is
				// not, and is not here — see PhoneRejectedError.
				h.logger.WarnContext(r.Context(), "auth: verified identifier is not a storable phone number",
					slog.String("reason", "phone_unusable"),
					slog.Int("phone_chars", pe.Digits),
					slog.String("phone_prefix", pe.Prefix),
					slog.String("request_id", httpx.RequestIDFrom(r.Context())))
			default:
				h.logger.WarnContext(r.Context(), "auth: session token rejected",
					slog.String("reason", "unspecified"),
					slog.Int("token_length", len(body.AccessToken)),
					slog.String("request_id", httpx.RequestIDFrom(r.Context())))
			}
			// Client always gets one message — which check failed is a security detail.
			httpx.WriteError(w, httpx.CodeUnauthorized,
				"We could not verify that sign-in. Please try again.")
		case errors.Is(err, ErrGatewayError):
			h.logger.ErrorContext(r.Context(), "auth: msg91 gateway error",
				slog.Any("error", err),
				slog.Int("token_length", len(body.AccessToken)),
				slog.String("request_id", httpx.RequestIDFrom(r.Context())))
			httpx.WriteError(w, httpx.CodeGatewayError,
				"We could not reach our sign-in provider. Please try again.")
		default:
			h.fail(r, w, "create session", err)
		}
		return
	}

	httpx.WriteData(w, http.StatusOK, verifyCodeResponse{
		Token:     result.Token,
		ExpiresAt: result.ExpiresAt,
		User:      toUserDTO(result.User),
	})
}

func (h *Handler) requestCode(w http.ResponseWriter, r *http.Request) {
	var body requestCodeBody
	if !decodeJSON(w, r, &body) {
		return
	}

	result, err := h.service.RequestCode(r.Context(), body.Phone)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidPhone):
			httpx.WriteFieldErrors(w, []httpx.FieldError{
				{Field: "phone", Message: "Enter a valid 10-digit mobile number"},
			})
		case errors.Is(err, ErrRateLimited):
			httpx.WriteError(w, httpx.CodeRateLimited, "Too many attempts. Try again in a minute.")
		default:
			h.fail(r, w, "request code", err)
		}
		return
	}

	httpx.WriteData(w, http.StatusAccepted, requestCodeResponse{
		ExpiresInSeconds:   int(result.ExpiresIn.Seconds()),
		ResendAfterSeconds: int(result.ResendAfter.Seconds()),
		DevCode:            result.DevCode,
	})
}

func (h *Handler) verifyCode(w http.ResponseWriter, r *http.Request) {
	var body verifyCodeBody
	if !decodeJSON(w, r, &body) {
		return
	}

	result, err := h.service.Verify(r.Context(), body.Phone, body.Code)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidPhone):
			httpx.WriteFieldErrors(w, []httpx.FieldError{
				{Field: "phone", Message: "Enter a valid 10-digit mobile number"},
			})
		case errors.Is(err, ErrInvalidCodeShape):
			httpx.WriteFieldErrors(w, []httpx.FieldError{
				{Field: "code", Message: "Enter the 6-digit code"},
			})
		case errors.Is(err, ErrCodeRejected):
			// One message for wrong, expired, spent and locked-out.
			httpx.WriteError(w, httpx.CodeUnauthorized, "That code is not correct or has expired.")
		default:
			h.fail(r, w, "verify code", err)
		}
		return
	}

	httpx.WriteData(w, http.StatusOK, verifyCodeResponse{
		Token:     result.Token,
		ExpiresAt: result.ExpiresAt,
		User:      toUserDTO(result.User),
	})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFrom(r.Context())
	if !ok {
		// Unreachable behind Require; guarded so a routing mistake cannot
		// silently serve an anonymous request.
		unauthorized(w)
		return
	}

	user, err := h.service.UserByID(r.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			// A valid token for a deleted account: the credential is stale.
			unauthorized(w)
			return
		}
		h.fail(r, w, "get user", err)
		return
	}

	httpx.WriteData(w, http.StatusOK, meResponse{User: toUserDTO(user)})
}

func (h *Handler) listAddresses(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFrom(r.Context())
	if !ok {
		unauthorized(w)
		return
	}

	addrs, err := h.service.Addresses(r.Context(), userID)
	if err != nil {
		h.addressFailure(w, r, "list addresses", err)
		return
	}
	if addrs == nil {
		addrs = []Address{}
	}
	httpx.WriteData(w, http.StatusOK, addressListResponse{Addresses: addrs})
}

func (h *Handler) addAddress(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFrom(r.Context())
	if !ok {
		unauthorized(w)
		return
	}

	var body Address
	if !decodeJSON(w, r, &body) {
		return
	}

	saved, problem, err := h.service.AddAddress(r.Context(), userID, body)
	if err != nil {
		h.addressFailure(w, r, "add address", err)
		return
	}
	if problem.Field != "" {
		httpx.WriteFieldErrors(w, []httpx.FieldError{{Field: problem.Field, Message: problem.Message}})
		return
	}
	httpx.WriteData(w, http.StatusCreated, addressResponse{Address: saved})
}

func (h *Handler) updateAddress(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFrom(r.Context())
	if !ok {
		unauthorized(w)
		return
	}

	addressID, ok := parseAddressID(w, mux.Vars(r)["addressId"])
	if !ok {
		return
	}

	var body Address
	if !decodeJSON(w, r, &body) {
		return
	}

	saved, problem, err := h.service.UpdateAddress(r.Context(), userID, addressID, body)
	if err != nil {
		h.addressFailure(w, r, "update address", err)
		return
	}
	if problem.Field != "" {
		httpx.WriteFieldErrors(w, []httpx.FieldError{{Field: problem.Field, Message: problem.Message}})
		return
	}
	httpx.WriteData(w, http.StatusOK, addressResponse{Address: saved})
}

func (h *Handler) deleteAddress(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFrom(r.Context())
	if !ok {
		unauthorized(w)
		return
	}

	addressID, ok := parseAddressID(w, mux.Vars(r)["addressId"])
	if !ok {
		return
	}

	remaining, err := h.service.DeleteAddress(r.Context(), userID, addressID)
	if err != nil {
		h.addressFailure(w, r, "delete address", err)
		return
	}
	if remaining == nil {
		remaining = []Address{}
	}
	httpx.WriteData(w, http.StatusOK, addressListResponse{Addresses: remaining})
}

// addressFailure maps the shared error cases for every address endpoint.
// An unknown address and someone else's address are both 404 — distinguishing
// them would leak which ids exist.
func (h *Handler) addressFailure(w http.ResponseWriter, r *http.Request, op string, err error) {
	switch {
	case errors.Is(err, ErrUserNotFound):
		unauthorized(w)
	case errors.Is(err, ErrAddressNotFound):
		httpx.WriteError(w, httpx.CodeNotFound, "That address does not exist.")
	default:
		h.fail(r, w, op, err)
	}
}

// parseAddressID rejects a malformed id as a 404 rather than a 400: from the
// caller's point of view an unparseable id is simply one that does not exist.
func parseAddressID(w http.ResponseWriter, raw string) (bson.ObjectID, bool) {
	id, err := bson.ObjectIDFromHex(raw)
	if err != nil {
		httpx.WriteError(w, httpx.CodeNotFound, "That address does not exist.")
		return bson.NilObjectID, false
	}
	return id, true
}

/* ------------------------------------------------------------------ helpers */

// decodeJSON reads a JSON body, answering 400 and reporting false if it cannot.
// Unknown fields are rejected so a typo in a client payload is not silently
// ignored.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		httpx.WriteError(w, httpx.CodeBadRequest, "The request body could not be read.")
		return false
	}
	return true
}

// maxBodyBytes caps request bodies. These payloads are two short strings; the
// limit stops a client streaming gigabytes into the decoder.
const maxBodyBytes = 4 << 10

// fail logs the cause and returns a generic 500. Codes, tokens and driver
// detail never reach the client.
func (h *Handler) fail(r *http.Request, w http.ResponseWriter, op string, err error) {
	h.logger.ErrorContext(r.Context(), "auth: "+op,
		slog.Any("error", err),
		slog.String("request_id", httpx.RequestIDFrom(r.Context())),
	)
	httpx.WriteError(w, httpx.CodeInternal, "Something went wrong on our side.")
}
