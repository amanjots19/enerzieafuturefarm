// Package auth owns sign-in: one-time codes, the user identity they create,
// and the bearer tokens that carry it.
//
// There are no passwords. A shopper's mobile number is the identity
// (product.md §3.3), proven by a short-lived code.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"regexp"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Code and phone shapes, matching what the frontend enforces.
const (
	// CodeLength is the number of digits in a one-time code.
	CodeLength = 6
	// PhoneLength is the digits in an Indian mobile number, without +91.
	PhoneLength = 10
)

// Policy values from roadmap.md §Auth.
const (
	// CodeTTL is how long a code stays usable.
	CodeTTL = 5 * time.Minute
	// MaxAttempts is how many wrong guesses a code tolerates before it locks.
	MaxAttempts = 5
	// TokenTTL is how long an issued bearer token stays valid.
	TokenTTL = 30 * 24 * time.Hour
	// ResendInterval is the minimum gap between two code requests.
	ResendInterval = 30 * time.Second
	// RateWindow and MaxRequestsPerWindow cap how often a number can ask for
	// a code.
	RateWindow           = 10 * time.Minute
	MaxRequestsPerWindow = 3
)

// phonePattern is exactly ten digits: no +91, no spaces, no punctuation. The
// frontend strips those before sending.
var phonePattern = regexp.MustCompile(`^\d{10}$`)

// ValidPhone reports whether s is a well-formed Indian mobile number.
func ValidPhone(s string) bool { return phonePattern.MatchString(s) }

// codePattern is exactly six digits.
var codePattern = regexp.MustCompile(`^\d{6}$`)

// ValidCodeShape reports whether s looks like a one-time code. It says nothing
// about whether the code is correct.
func ValidCodeShape(s string) bool { return codePattern.MatchString(s) }

// Address is one of a shopper's delivery addresses, embedded in their user
// document (schema.md §users). It carries its own id so the API can address a
// single entry without positional index games.
//
// Email lives here because the UI asks for it as an address for order updates,
// not as a login — two addresses may carry different ones.
type Address struct {
	ID    bson.ObjectID `bson:"_id"   json:"id"`
	Label string        `bson:"label,omitempty" json:"label,omitempty"`
	Name  string        `bson:"name"  json:"name"`
	Email string        `bson:"email" json:"email"`
	Line1 string        `bson:"line1" json:"line1"`
	City  string        `bson:"city"  json:"city"`
	State string        `bson:"state" json:"state"`
	Pin   string        `bson:"pin"   json:"pin"`
	// IsDefault marks the address pre-selected at checkout. Exactly one entry
	// carries it whenever the list is non-empty.
	IsDefault bool `bson:"isDefault" json:"isDefault"`
}

// User is a shopper, created on their first successful code verification.
type User struct {
	ID        bson.ObjectID `bson:"_id,omitempty"`
	Phone     string        `bson:"phone"`
	Addresses []Address     `bson:"addresses,omitempty"`
	CreatedAt time.Time     `bson:"createdAt"`
	UpdatedAt time.Time     `bson:"updatedAt"`
}

// OTPCode is a pending sign-in challenge (schema.md §otp_codes). The code
// itself is never stored — only a keyed hash of it.
type OTPCode struct {
	ID        bson.ObjectID `bson:"_id,omitempty"`
	Phone     string        `bson:"phone"`
	CodeHash  string        `bson:"codeHash"`
	Attempts  int           `bson:"attempts"`
	Consumed  bool          `bson:"consumed"`
	CreatedAt time.Time     `bson:"createdAt"`
	ExpiresAt time.Time     `bson:"expiresAt"`
}

// Usable reports whether a code can still be verified: not spent, not expired,
// and not locked out by repeated wrong guesses.
func (c OTPCode) Usable(now time.Time) bool {
	return !c.Consumed && c.Attempts < MaxAttempts && now.Before(c.ExpiresAt)
}

// GenerateCode returns a cryptographically random six-digit code. Leading
// zeros are preserved, so "004213" is a valid code and the space really is
// 10^6.
func GenerateCode() (string, error) {
	limit := big.NewInt(1_000_000)
	n, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return "", err
	}
	// %06d keeps the width fixed; without it small numbers would be guessable
	// from their length alone.
	return pad6(n.Int64()), nil
}

func pad6(n int64) string {
	const digits = "0123456789"
	out := make([]byte, CodeLength)
	for i := CodeLength - 1; i >= 0; i-- {
		out[i] = digits[n%10]
		n /= 10
	}
	return string(out)
}

// HashCode returns the value stored for a code.
//
// This is an HMAC keyed with a server-side pepper, not a bare hash. A plain
// SHA-256 of a six-digit code is reversible instantly — the whole keyspace is
// a million entries — so anyone who could read the collection could sign in as
// anybody. Keying it means the stored value is useless without the secret.
//
// The phone number is mixed in so a hash cannot be replayed against a
// different number.
func HashCode(pepper []byte, phone, code string) string {
	mac := hmac.New(sha256.New, pepper)
	// Domain separation: this pepper also signs tokens.
	mac.Write([]byte("otp:" + phone + ":" + code))
	return hex.EncodeToString(mac.Sum(nil))
}

// CodeMatches compares a candidate code against a stored hash in constant
// time, so a timing signal cannot leak how much of a guess was right.
func CodeMatches(pepper []byte, phone, code, storedHash string) bool {
	return hmac.Equal([]byte(HashCode(pepper, phone, code)), []byte(storedHash))
}
