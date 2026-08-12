// Package msg91 wraps the MSG91 widget token-verification API.
//
// The only contract this package owns is the HTTP call to
// POST /api/v5/widget/verifyAccessToken. Normalising the returned phone
// number and mapping errors to session outcomes are the caller's job.
package msg91

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// codeField unmarshals MSG91's code field, which may be a JSON string or number.
// We strip JSON string delimiters so both "418" and 418 decode to the same value.
type codeField string

func (c *codeField) UnmarshalJSON(data []byte) error {
	*c = codeField(strings.Trim(string(data), `"`))
	return nil
}

// ErrNotConfigured is returned by Unconfigured on every method call.
var ErrNotConfigured = errors.New("msg91: no verifier configured")

// ErrVerificationFailed is the sentinel wrapped by VerificationError.
// Use errors.Is to test for rejection; use errors.As to access diagnostic detail.
//
// TRAP: MSG91 can return HTTP 200 with type "error". The HTTP status alone is
// not sufficient to determine success — the type field is authoritative.
var ErrVerificationFailed = errors.New("msg91: token verification failed")

// VerificationError is returned when MSG91 returns type != "success". It
// carries MSG91's diagnostic fields (Type, Code, Message) so the caller can
// log them without returning them to clients. It wraps ErrVerificationFailed
// so existing errors.Is checks keep working.
type VerificationError struct {
	// Type is MSG91's type field (e.g. "error").
	Type string
	// Code is MSG91's error code (e.g. "418" for AuthenticationFailure).
	// Empty when absent from the response.
	Code string
	// Message is MSG91's human-readable reason (e.g. "AuthenticationFailure").
	// This is a diagnostic string from MSG91, not a secret, and is safe to log.
	Message string
}

// Error implements the error interface.
func (e *VerificationError) Error() string {
	return fmt.Sprintf("msg91: verification failed: type=%q code=%q message=%q",
		e.Type, e.Code, e.Message)
}

// Unwrap returns ErrVerificationFailed so that errors.Is(err, ErrVerificationFailed)
// is true for any *VerificationError.
func (e *VerificationError) Unwrap() error { return ErrVerificationFailed }

// Verifier verifies an MSG91 widget access token.
type Verifier interface {
	// VerifyAccessToken calls MSG91 and returns the raw phone string MSG91
	// reported on success, e.g. "919999999999". Normalising that to the
	// stored 10-digit form is the caller's responsibility.
	VerifyAccessToken(ctx context.Context, accessToken string) (phone string, err error)
}

// Client is the real HTTP implementation of Verifier.
type Client struct {
	baseURL    string
	authKey    string
	httpClient *http.Client
}

// NewClient returns a Client targeting MSG91's production API.
func NewClient(authKey string) *Client {
	return newClient(authKey, "https://api.msg91.com")
}

// NewClientWithBaseURL returns a Client with a custom base URL. Use this in
// tests to point the client at an httptest.Server — the live API is never
// called from a test.
func NewClientWithBaseURL(authKey, baseURL string) *Client {
	return newClient(authKey, baseURL)
}

func newClient(authKey, baseURL string) *Client {
	// MSG91 enforces an IP allowlist on the authkey and only our IPv4 egress
	// address is registered in the dashboard. Force tcp4 so every call leaves
	// from that known address. IPv6 SLAAC / privacy extensions rotate the
	// address, making an IPv6 allowlist entry fragile. Do not remove this dialer
	// or widen it to the global default — nothing else in the service has an
	// IP-allowlisted upstream, and a global change would silently affect the
	// Mongo and Razorpay clients too.
	dialer := &net.Dialer{}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, "tcp4", addr)
		},
	}
	return &Client{
		baseURL: baseURL,
		authKey: authKey,
		httpClient: &http.Client{
			Timeout:   10 * time.Second,
			Transport: transport,
		},
	}
}

type verifyRequest struct {
	AccessToken string `json:"access-token"`
}

// verifyResponse is MSG91's reply shape. Type is the authoritative success
// signal — not the HTTP status code. Code is present on error responses.
// MSG91 sends code as either a JSON string or a number depending on the error;
// codeField handles both forms.
type verifyResponse struct {
	Type    string    `json:"type"`
	Code    codeField `json:"code"`
	Message string    `json:"message"`
}

// VerifyAccessToken calls MSG91 to check the widget access token and returns
// the raw phone string. The authkey header carries the server-only secret and
// must never be logged.
func (c *Client) VerifyAccessToken(ctx context.Context, accessToken string) (string, error) {
	body, _ := json.Marshal(verifyRequest{AccessToken: accessToken}) //nolint:gosec // G117: intentionally marshaling the access-token field into the outbound MSG91 request body

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/v5/widget/verifyAccessToken", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("msg91: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("authkey", c.authKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("msg91: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("msg91: unexpected status %d", resp.StatusCode)
	}

	var result verifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("msg91: decode response: %w", err)
	}

	// Success is type == "success", not the HTTP status. A 200 with
	// type "error" is still a failed verification. Return a typed error so
	// the caller can log MSG91's code and message for diagnostics.
	if result.Type != "success" {
		return "", &VerificationError{Type: result.Type, Code: string(result.Code), Message: result.Message}
	}
	return result.Message, nil
}

// Unconfigured implements Verifier and returns ErrNotConfigured on every call.
// It is the default when MSG91_AUTH_KEY is absent.
type Unconfigured struct{}

// VerifyAccessToken always returns ErrNotConfigured.
func (Unconfigured) VerifyAccessToken(context.Context, string) (string, error) {
	return "", ErrNotConfigured
}
