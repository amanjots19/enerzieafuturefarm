package cloudinary_test

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"

	"github.com/enerzia/enerzia-be/internal/cloudinary"
)

// TestClientSignUploadKnownDigest verifies that a known set of inputs produces
// the correct SHA-1 digest.
//
// The expected digest is an external reference value computed outside Go with
// `shasum -a 1` from the documented algorithm:
//
//	SHA-1("folder=enerzia/products&timestamp=1700000000" + apiSecret)
//
// Do NOT regenerate this constant from our own output — a test that asserts the
// code agrees with itself proves nothing. If SignUpload changes its format and
// the constant disagrees, that disagreement IS the finding.
func TestClientSignUploadKnownDigest(t *testing.T) {
	const (
		cloudName = "testcloud"
		apiKey    = "123456789012345"
		apiSecret = "abcdef1234567890abcdef12"
		folder    = "enerzia/products"
		timestamp = int64(1700000000)
		// External reference: shasum -a 1 <<< "folder=enerzia/products&timestamp=1700000000abcdef1234567890abcdef12"
		want = "6a01201b2427bf1ffc6b776b9cd34c00e091bdd4"
	)

	client := cloudinary.NewClient(cloudName, apiKey, apiSecret, folder)
	sig, err := client.SignUpload(timestamp)
	if err != nil {
		t.Fatalf("SignUpload() error = %v", err)
	}

	if sig.Signature != want {
		t.Errorf("Signature = %q, want %q", sig.Signature, want)
	}
	if sig.CloudName != cloudName {
		t.Errorf("CloudName = %q, want %q", sig.CloudName, cloudName)
	}
	if sig.APIKey != apiKey {
		t.Errorf("APIKey = %q, want %q", sig.APIKey, apiKey)
	}
	if sig.Timestamp != timestamp {
		t.Errorf("Timestamp = %d, want %d", sig.Timestamp, timestamp)
	}
	if sig.Folder != folder {
		t.Errorf("Folder = %q, want %q", sig.Folder, folder)
	}
}

// TestClientSignUploadSortsByKey verifies that parameters are sorted by KEY
// alphabetically, not by value. The folder value starts with 'z', which sorts
// after 't' by value — but "folder" the key sorts before "timestamp", so
// folder must still appear first in the signed string.
//
// wantCorrect is an external reference value computed outside Go with
// `shasum -a 1` from the documented algorithm:
//
//	SHA-1("folder=zone/uploads&timestamp=1700000000" + apiSecret)
//
// Do NOT regenerate this constant from our own output.
func TestClientSignUploadSortsByKey(t *testing.T) {
	const (
		cloudName = "testcloud"
		apiKey    = "key123"
		apiSecret = "topsecret"
		folder    = "zone/uploads" // 'z' > 't' by value; key "folder" < "timestamp" alphabetically
		timestamp = int64(1700000000)
		// External reference: shasum -a 1 <<< "folder=zone/uploads&timestamp=1700000000topsecret"
		wantCorrect = "93ecdb5806c902af85e7447af07fec78e7b15948"
	)

	client := cloudinary.NewClient(cloudName, apiKey, apiSecret, folder)
	sig, err := client.SignUpload(timestamp)
	if err != nil {
		t.Fatalf("SignUpload() error = %v", err)
	}

	// Compute what the signature would be with the WRONG parameter order
	// (timestamp before folder). This proves the two orderings are distinct and
	// that our constant is testing the right thing.
	hWrong := sha1.New()
	fmt.Fprintf(hWrong, "timestamp=%d&folder=%s%s", timestamp, folder, apiSecret)
	wantWrong := hex.EncodeToString(hWrong.Sum(nil))

	if sig.Signature != wantCorrect {
		t.Errorf("Signature = %q, want key-alphabetical order (folder before timestamp): %q", sig.Signature, wantCorrect)
	}
	if sig.Signature == wantWrong {
		t.Error("Signature matches wrong parameter order (timestamp before folder); key sorting is broken")
	}
}

// TestClientSignUploadDifferentTimestamps verifies that different timestamps
// produce different signatures, confirming the timestamp is part of the signed
// payload.
func TestClientSignUploadDifferentTimestamps(t *testing.T) {
	client := cloudinary.NewClient("cloud", "key", "secret", "folder/a")

	sig1, err := client.SignUpload(1700000000)
	if err != nil {
		t.Fatalf("SignUpload(t1) error = %v", err)
	}
	sig2, err := client.SignUpload(1700000001)
	if err != nil {
		t.Fatalf("SignUpload(t2) error = %v", err)
	}

	if sig1.Signature == sig2.Signature {
		t.Error("different timestamps produced identical signatures; timestamp is not included in signing")
	}
}

// TestClientDefaultFolder verifies that NewClient defaults the folder to
// "enerzia/products" when an empty string is passed.
func TestClientDefaultFolder(t *testing.T) {
	client := cloudinary.NewClient("cloud", "key", "secret", "")
	sig, err := client.SignUpload(1700000000)
	if err != nil {
		t.Fatalf("SignUpload() error = %v", err)
	}
	if sig.Folder != "enerzia/products" {
		t.Errorf("Folder = %q, want %q", sig.Folder, "enerzia/products")
	}
}

// TestUnconfiguredReturnsErrNotConfigured verifies that Unconfigured.SignUpload
// returns ErrNotConfigured.
func TestUnconfiguredReturnsErrNotConfigured(t *testing.T) {
	var u cloudinary.Unconfigured
	_, err := u.SignUpload(1700000000)
	if !errors.Is(err, cloudinary.ErrNotConfigured) {
		t.Errorf("Unconfigured.SignUpload() error = %v, want ErrNotConfigured", err)
	}
}
