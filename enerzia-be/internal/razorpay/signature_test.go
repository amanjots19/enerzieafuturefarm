package razorpay

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
)

// makeCallbackSig returns a valid hex-encoded HMAC-SHA256 callback signature.
func makeCallbackSig(orderID, paymentID, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(orderID + "|" + paymentID))
	return hex.EncodeToString(mac.Sum(nil))
}

// makeWebhookSig returns a valid hex-encoded HMAC-SHA256 webhook signature.
func makeWebhookSig(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestVerifyCallbackSignature(t *testing.T) {
	const (
		orderID   = "order_test123"
		paymentID = "pay_test456"
		secret    = "test-key-secret-that-is-long-enough-x"
	)

	tests := []struct {
		name      string
		orderID   string
		paymentID string
		sig       string
		wantErr   bool
	}{
		{
			name:      "valid",
			orderID:   orderID,
			paymentID: paymentID,
			sig:       makeCallbackSig(orderID, paymentID, secret),
		},
		{
			name:      "wrong order id",
			orderID:   "order_other",
			paymentID: paymentID,
			sig:       makeCallbackSig(orderID, paymentID, secret),
			wantErr:   true,
		},
		{
			name:      "wrong payment id",
			orderID:   orderID,
			paymentID: "pay_other",
			sig:       makeCallbackSig(orderID, paymentID, secret),
			wantErr:   true,
		},
		{
			name:      "all-zero hex signature",
			orderID:   orderID,
			paymentID: paymentID,
			sig:       "0000000000000000000000000000000000000000000000000000000000000000",
			wantErr:   true,
		},
		{
			name:      "non-hex signature",
			orderID:   orderID,
			paymentID: paymentID,
			sig:       "not-hex!!!",
			wantErr:   true,
		},
		{
			name:      "empty signature",
			orderID:   orderID,
			paymentID: paymentID,
			sig:       "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifyCallbackSignature(tt.orderID, tt.paymentID, tt.sig, secret)
			if tt.wantErr {
				if err == nil {
					t.Error("want error, got nil")
				} else if !errors.Is(err, ErrBadSignature) {
					t.Errorf("want ErrBadSignature, got %v", err)
				}
			} else if err != nil {
				t.Errorf("want nil, got %v", err)
			}
		})
	}
}

func TestVerifyWebhookSignature(t *testing.T) {
	const secret = "test-webhook-secret-long-enough-yy"
	body := []byte(`{"event":"payment.captured","payload":{"payment":{"entity":{"id":"pay_test456"}}}}`)

	tests := []struct {
		name    string
		body    []byte
		sig     string
		wantErr bool
	}{
		{
			name: "valid",
			body: body,
			sig:  makeWebhookSig(body, secret),
		},
		{
			name:    "tampered body",
			body:    []byte(`{"event":"payment.captured","payload":{"payment":{"entity":{"id":"pay_HACKED"}}}}`),
			sig:     makeWebhookSig(body, secret),
			wantErr: true,
		},
		{
			name:    "all-zero hex signature",
			body:    body,
			sig:     "0000000000000000000000000000000000000000000000000000000000000000",
			wantErr: true,
		},
		{
			name:    "non-hex signature",
			body:    body,
			sig:     "not-hex!!!",
			wantErr: true,
		},
		{
			name:    "empty signature",
			body:    body,
			sig:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifyWebhookSignature(tt.body, tt.sig, secret)
			if tt.wantErr {
				if err == nil {
					t.Error("want error, got nil")
				} else if !errors.Is(err, ErrBadSignature) {
					t.Errorf("want ErrBadSignature, got %v", err)
				}
			} else if err != nil {
				t.Errorf("want nil, got %v", err)
			}
		})
	}
}

// TestCrossSecretRejection proves that a valid signature for one verifier is
// always rejected by the other verifier, because the two secrets are different.
// This is the primary security property of the two-secret design.
func TestCrossSecretRejection(t *testing.T) {
	const (
		keySecret     = "key-secret-aaaaaaaaaaaaaaaaaaaaaaaa"
		webhookSecret = "webhook-secret-bbbbbbbbbbbbbbbbbbbb"
		orderID       = "order_abc123"
		paymentID     = "pay_xyz789"
	)
	body := []byte(`{"event":"payment.captured"}`)

	t.Run("callback verifier rejects a webhook signature", func(t *testing.T) {
		// Build a legitimately correct webhook signature using webhookSecret.
		sig := makeWebhookSig(body, webhookSecret)

		// Present it to the callback verifier, which uses keySecret and a
		// different payload format. This must always fail.
		if err := verifyCallbackSignature(orderID, paymentID, sig, keySecret); err == nil {
			t.Fatal("callback verifier accepted a webhook signature — cross-secret attack succeeded")
		}
	})

	t.Run("webhook verifier rejects a callback signature", func(t *testing.T) {
		// Build a legitimately correct callback signature using keySecret.
		sig := makeCallbackSig(orderID, paymentID, keySecret)

		// Present it to the webhook verifier, which uses webhookSecret and a
		// different payload (the raw body). This must always fail.
		if err := verifyWebhookSignature(body, sig, webhookSecret); err == nil {
			t.Fatal("webhook verifier accepted a callback signature — cross-secret attack succeeded")
		}
	})
}
