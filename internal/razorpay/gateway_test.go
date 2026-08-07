package razorpay_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/enerzia/enerzia-be/internal/razorpay"
)

func TestNewClient(t *testing.T) {
	c := razorpay.NewClient("rzp_live_id", "key_secret", "webhook_secret")

	const wantBaseURL = "https://api.razorpay.com/v1"
	if got := razorpay.ClientBaseURL(c); got != wantBaseURL {
		t.Errorf("NewClient() base URL = %q, want %q", got, wantBaseURL)
	}

	const wantTimeout = 10 * time.Second
	if got := razorpay.ClientTimeout(c); got != wantTimeout {
		t.Errorf("NewClient() timeout = %v, want %v", got, wantTimeout)
	}
}

func TestUnconfigured(t *testing.T) {
	var gw razorpay.Unconfigured

	t.Run("CreateOrder", func(t *testing.T) {
		_, err := gw.CreateOrder(context.Background(), razorpay.CreateOrderRequest{})
		if !errors.Is(err, razorpay.ErrNotConfigured) {
			t.Errorf("got %v, want ErrNotConfigured", err)
		}
	})

	t.Run("VerifyCallbackSignature", func(t *testing.T) {
		err := gw.VerifyCallbackSignature("", "", "")
		if !errors.Is(err, razorpay.ErrNotConfigured) {
			t.Errorf("got %v, want ErrNotConfigured", err)
		}
	})

	t.Run("VerifyWebhookSignature", func(t *testing.T) {
		err := gw.VerifyWebhookSignature(nil, "")
		if !errors.Is(err, razorpay.ErrNotConfigured) {
			t.Errorf("got %v, want ErrNotConfigured", err)
		}
	})
}

func TestClientCreateOrder(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.URL.Path != "/orders" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(razorpay.Order{
				ID:       "order_test123",
				Amount:   49900,
				Currency: "INR",
				Receipt:  "EFF-000001",
				Status:   "created",
			})
		}))
		defer srv.Close()

		c := razorpay.NewClientWithBaseURL("rzp_test_id", "key_secret", "webhook_secret", srv.URL)
		order, err := c.CreateOrder(context.Background(), razorpay.CreateOrderRequest{
			Amount:   49900,
			Currency: "INR",
			Receipt:  "EFF-000001",
		})
		if err != nil {
			t.Fatalf("CreateOrder() error = %v, want nil", err)
		}
		if order.ID != "order_test123" {
			t.Errorf("order.ID = %q, want %q", order.ID, "order_test123")
		}
		if order.Amount != 49900 {
			t.Errorf("order.Amount = %d, want 49900", order.Amount)
		}
	})

	t.Run("api error returns code not body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"code":        "BAD_REQUEST_ERROR",
					"description": "amount must be at least 100 paise — confidential detail",
				},
			})
		}))
		defer srv.Close()

		c := razorpay.NewClientWithBaseURL("rzp_test_id", "key_secret", "webhook_secret", srv.URL)
		_, err := c.CreateOrder(context.Background(), razorpay.CreateOrderRequest{
			Amount: 1, Currency: "INR", Receipt: "EFF-000001",
		})
		if err == nil {
			t.Fatal("CreateOrder() error = nil, want an error")
		}
		if !strings.Contains(err.Error(), "BAD_REQUEST_ERROR") {
			t.Errorf("error %q should contain the Razorpay error code", err)
		}
		// The body description must never appear in our error.
		if strings.Contains(err.Error(), "confidential detail") {
			t.Errorf("error %q must not echo the Razorpay response body", err)
		}
	})

	t.Run("non-json error body falls back to http status code", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("internal server error"))
		}))
		defer srv.Close()

		c := razorpay.NewClientWithBaseURL("rzp_test_id", "key_secret", "webhook_secret", srv.URL)
		_, err := c.CreateOrder(context.Background(), razorpay.CreateOrderRequest{
			Amount: 49900, Currency: "INR",
		})
		if err == nil {
			t.Fatal("CreateOrder() error = nil, want an error")
		}
		if !strings.Contains(err.Error(), "http_500") {
			t.Errorf("error %q should contain http_500", err)
		}
	})

	t.Run("cancelled context propagates", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		c := razorpay.NewClientWithBaseURL("rzp_test_id", "key_secret", "webhook_secret", srv.URL)
		_, err := c.CreateOrder(ctx, razorpay.CreateOrderRequest{Amount: 49900, Currency: "INR"})
		if err == nil {
			t.Error("CreateOrder() error = nil, want error from cancelled context")
		}
	})

	t.Run("200 with malformed json returns decode error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id": "order_123", "amount": NOT_JSON`))
		}))
		defer srv.Close()

		c := razorpay.NewClientWithBaseURL("rzp_test_id", "key_secret", "webhook_secret", srv.URL)
		order, err := c.CreateOrder(context.Background(), razorpay.CreateOrderRequest{
			Amount: 49900, Currency: "INR",
		})
		if err == nil {
			t.Fatal("CreateOrder() error = nil, want a decode error")
		}
		// A decode error must never silently return a partial Order — an empty
		// razorpayOrderID stored on the order document would be undetectable later.
		if order.ID != "" {
			t.Errorf("order.ID = %q on error, want empty", order.ID)
		}
		if !strings.Contains(err.Error(), "decode response") {
			t.Errorf("error %q should mention decode response", err)
		}
	})

	t.Run("invalid base url returns build request error", func(t *testing.T) {
		// "http://[::1" has an unclosed IPv6 bracket — url.Parse returns an
		// error, exercising the http.NewRequestWithContext failure branch.
		c := razorpay.NewClientWithBaseURL("rzp_test_id", "key_secret", "webhook_secret", "http://[::1")
		_, err := c.CreateOrder(context.Background(), razorpay.CreateOrderRequest{
			Amount: 49900, Currency: "INR",
		})
		if err == nil {
			t.Fatal("CreateOrder() error = nil, want error from invalid URL")
		}
		if !strings.Contains(err.Error(), "build request") {
			t.Errorf("error %q should mention build request", err)
		}
	})
}

// TestClientVerifyDelegates checks that Client.Verify* methods are wired to the
// correct secrets by confirming that malformed signatures are always rejected.
// The full correctness and cross-secret rejection tests live in signature_test.go.
func TestClientVerifyDelegates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := razorpay.NewClientWithBaseURL("rzp_test", "key-secret", "webhook-secret", srv.URL)

	t.Run("VerifyCallbackSignature rejects bad sig", func(t *testing.T) {
		tests := []string{"not-hex!!!", "", "0000000000000000000000000000000000000000000000000000000000000000"}
		for _, sig := range tests {
			if err := c.VerifyCallbackSignature("order_abc", "pay_xyz", sig); err == nil {
				t.Errorf("sig=%q: want error, got nil", sig)
			}
		}
	})

	t.Run("VerifyWebhookSignature rejects bad sig", func(t *testing.T) {
		body := []byte(`{"event":"payment.captured"}`)
		tests := []string{"not-hex!!!", "", "0000000000000000000000000000000000000000000000000000000000000000"}
		for _, sig := range tests {
			if err := c.VerifyWebhookSignature(body, sig); err == nil {
				t.Errorf("sig=%q: want error, got nil", sig)
			}
		}
	})
}
