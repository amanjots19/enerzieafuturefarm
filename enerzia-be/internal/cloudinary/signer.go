// Package cloudinary provides signed-upload support for the Cloudinary
// media management API.
package cloudinary

import "errors"

// ErrNotConfigured is returned by Unconfigured on every call.
var ErrNotConfigured = errors.New("cloudinary: not configured")

// Signature is the payload returned by Signer.SignUpload. It contains every
// value the browser needs to perform a signed direct upload to Cloudinary.
// The API secret is never included — it stays server-side.
type Signature struct {
	CloudName string `json:"cloudName"`
	APIKey    string `json:"apiKey"`
	Timestamp int64  `json:"timestamp"`
	Folder    string `json:"folder"`
	Signature string `json:"signature"`
}

// Signer generates a signed-upload payload for the Cloudinary upload API.
type Signer interface {
	// SignUpload returns a Signature for a direct upload at the given Unix
	// timestamp. The signature is a SHA-1 hex digest of the sorted parameters
	// with the API secret appended, as Cloudinary's documentation specifies.
	SignUpload(timestamp int64) (Signature, error)
}

// Unconfigured implements Signer and returns ErrNotConfigured on every call.
// It is the default when CLOUDINARY_CLOUD_NAME, CLOUDINARY_API_KEY, or
// CLOUDINARY_API_SECRET are absent from the environment.
type Unconfigured struct{}

// SignUpload always returns ErrNotConfigured.
func (Unconfigured) SignUpload(int64) (Signature, error) {
	return Signature{}, ErrNotConfigured
}
