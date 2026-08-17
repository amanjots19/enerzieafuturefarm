package cloudinary

import (
	"crypto/sha1" //nolint:gosec // Cloudinary mandates SHA-1; substituting a stronger hash breaks the signing scheme.
	"encoding/hex"
	"fmt"
	"io"
)

// Client implements Signer for a real Cloudinary account.
type Client struct {
	cloudName string
	apiKey    string
	apiSecret string
	folder    string
}

// NewClient returns a Client for the given Cloudinary account.
// cloudName, apiKey, and apiSecret are required. folder defaults to
// "enerzia/products" when empty.
func NewClient(cloudName, apiKey, apiSecret, folder string) *Client {
	if folder == "" {
		folder = "enerzia/products"
	}
	return &Client{
		cloudName: cloudName,
		apiKey:    apiKey,
		apiSecret: apiSecret,
		folder:    folder,
	}
}

// SignUpload returns a signed-upload payload for the given Unix timestamp.
//
// The signature is the SHA-1 hex digest of:
//
//	folder=<folder>&timestamp=<timestamp><apiSecret>
//
// Parameters are sorted alphabetically by key ("folder" before "timestamp"),
// and the API secret is appended directly — no HMAC, no separator — as
// Cloudinary's authenticated-request documentation specifies.
func (c *Client) SignUpload(timestamp int64) (Signature, error) {
	// "folder" (f) precedes "timestamp" (t) in alphabetical key order.
	params := fmt.Sprintf("folder=%s&timestamp=%d", c.folder, timestamp)
	payload := params + c.apiSecret

	h := sha1.New() //nolint:gosec // Cloudinary mandates SHA-1; substituting a stronger hash breaks the signing scheme.
	_, _ = io.WriteString(h, payload)

	return Signature{
		CloudName: c.cloudName,
		APIKey:    c.apiKey,
		Timestamp: timestamp,
		Folder:    c.folder,
		Signature: hex.EncodeToString(h.Sum(nil)),
	}, nil
}
