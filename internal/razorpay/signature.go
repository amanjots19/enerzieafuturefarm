package razorpay

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

// ErrBadSignature is returned when a signature does not match the expected HMAC.
var ErrBadSignature = errors.New("razorpay: signature mismatch")

// verifyCallbackSignature checks the HMAC-SHA256 of
// razorpayOrderID + "|" + razorpayPaymentID against keySecret.
// This is the Razorpay payment verification flow (callback from the JS SDK).
// keySecret and webhookSecret are different — they must not be swapped.
func verifyCallbackSignature(razorpayOrderID, razorpayPaymentID, signature, keySecret string) error {
	sig, err := hex.DecodeString(signature)
	if err != nil {
		return ErrBadSignature
	}

	mac := hmac.New(sha256.New, []byte(keySecret))
	mac.Write([]byte(razorpayOrderID + "|" + razorpayPaymentID))

	if !hmac.Equal(sig, mac.Sum(nil)) {
		return ErrBadSignature
	}
	return nil
}

// verifyWebhookSignature checks the HMAC-SHA256 of rawBody against
// webhookSecret. rawBody must be the exact bytes received from Razorpay,
// read before any JSON parsing.
// webhookSecret and keySecret are different — they must not be swapped.
func verifyWebhookSignature(rawBody []byte, signature, webhookSecret string) error {
	sig, err := hex.DecodeString(signature)
	if err != nil {
		return ErrBadSignature
	}

	mac := hmac.New(sha256.New, []byte(webhookSecret))
	mac.Write(rawBody)

	if !hmac.Equal(sig, mac.Sum(nil)) {
		return ErrBadSignature
	}
	return nil
}
