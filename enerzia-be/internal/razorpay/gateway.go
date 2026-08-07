// Package razorpay wraps the Razorpay Orders API and provides signature
// verification for payment callbacks and webhooks.
package razorpay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// ErrNotConfigured is returned by Unconfigured on every method call.
var ErrNotConfigured = errors.New("razorpay: no gateway configured")

// CreateOrderRequest is the body sent to POST /v1/orders.
type CreateOrderRequest struct {
	Amount   int64  `json:"amount"`   // paise, e.g. 49900 for ₹499
	Currency string `json:"currency"` // always "INR"
	Receipt  string `json:"receipt"`  // our EFF-###### order id
}

// Order is the response returned by Razorpay from POST /v1/orders.
type Order struct {
	ID       string `json:"id"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
	Receipt  string `json:"receipt"`
	Status   string `json:"status"`
}

// Gateway is the Razorpay integration surface used by the order service.
type Gateway interface {
	// CreateOrder reserves a payment on Razorpay. The returned Order.ID is
	// the razorpayOrderID stored on our order document.
	CreateOrder(ctx context.Context, req CreateOrderRequest) (Order, error)

	// VerifyCallbackSignature checks the HMAC sent by Razorpay's frontend
	// SDK after a successful payment. Uses the key secret.
	VerifyCallbackSignature(razorpayOrderID, razorpayPaymentID, signature string) error

	// VerifyWebhookSignature checks the X-Razorpay-Signature header on an
	// inbound webhook. Uses the webhook secret (different from the key secret).
	VerifyWebhookSignature(rawBody []byte, signature string) error
}

// Client is the real HTTP implementation of Gateway.
type Client struct {
	baseURL       string
	keyID         string
	keySecret     string
	webhookSecret string
	httpClient    *http.Client
}

// NewClient returns a Client targeting Razorpay's production API.
func NewClient(keyID, keySecret, webhookSecret string) *Client {
	return newClient(keyID, keySecret, webhookSecret, "https://api.razorpay.com/v1")
}

// NewClientWithBaseURL returns a Client with a custom base URL. Use this in
// tests to point the client at an httptest.Server.
func NewClientWithBaseURL(keyID, keySecret, webhookSecret, baseURL string) *Client {
	return newClient(keyID, keySecret, webhookSecret, baseURL)
}

func newClient(keyID, keySecret, webhookSecret, baseURL string) *Client {
	return &Client{
		baseURL:       baseURL,
		keyID:         keyID,
		keySecret:     keySecret,
		webhookSecret: webhookSecret,
		httpClient:    &http.Client{Timeout: 10 * time.Second},
	}
}

// CreateOrder calls POST /v1/orders on the Razorpay API and returns the
// created order. On a non-200 response the error carries only the Razorpay
// error code, never the request body or any secret.
func (c *Client) CreateOrder(ctx context.Context, req CreateOrderRequest) (Order, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return Order{}, fmt.Errorf("razorpay: create order: marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/orders", bytes.NewReader(body))
	if err != nil {
		return Order{}, fmt.Errorf("razorpay: create order: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.SetBasicAuth(c.keyID, c.keySecret)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return Order{}, fmt.Errorf("razorpay: create order: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var apiErr struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		code := apiErr.Error.Code
		if code == "" {
			code = fmt.Sprintf("http_%d", resp.StatusCode)
		}
		// Only the error code is reported; request body and secrets are never included.
		return Order{}, fmt.Errorf("razorpay: create order: %s", code)
	}

	var order Order
	if err := json.NewDecoder(resp.Body).Decode(&order); err != nil {
		return Order{}, fmt.Errorf("razorpay: create order: decode response: %w", err)
	}
	return order, nil
}

// VerifyCallbackSignature verifies the HMAC-SHA256 sent by Razorpay's JS SDK
// after payment. It uses the key secret, not the webhook secret.
func (c *Client) VerifyCallbackSignature(razorpayOrderID, razorpayPaymentID, signature string) error {
	return verifyCallbackSignature(razorpayOrderID, razorpayPaymentID, signature, c.keySecret)
}

// VerifyWebhookSignature verifies the X-Razorpay-Signature header on a
// webhook delivery. It uses the webhook secret, not the key secret.
func (c *Client) VerifyWebhookSignature(rawBody []byte, signature string) error {
	return verifyWebhookSignature(rawBody, signature, c.webhookSecret)
}

// Unconfigured implements Gateway and returns ErrNotConfigured on every call.
// It is the default in non-production deployments that have no Razorpay keys.
type Unconfigured struct{}

// CreateOrder always returns ErrNotConfigured.
func (Unconfigured) CreateOrder(context.Context, CreateOrderRequest) (Order, error) {
	return Order{}, ErrNotConfigured
}

// VerifyCallbackSignature always returns ErrNotConfigured.
func (Unconfigured) VerifyCallbackSignature(string, string, string) error {
	return ErrNotConfigured
}

// VerifyWebhookSignature always returns ErrNotConfigured.
func (Unconfigured) VerifyWebhookSignature([]byte, string) error {
	return ErrNotConfigured
}
