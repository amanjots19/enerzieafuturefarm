package order

import (
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"regexp"
)

// OrderIDDigits is the number of digits after the "EFF-" prefix.
const OrderIDDigits = 6

// orderIDSpace is the exclusive upper bound for the numeric part: the full
// six-digit space is 000000–999999, i.e. one million values. Drawing uniformly
// over exactly this range is what keeps the generator free of modulo bias.
var orderIDSpace = big.NewInt(1_000_000)

// orderIDPattern is the customer-facing id format: "EFF-" followed by exactly
// six digits. Leading zeros are admitted, so "EFF-004213" is valid.
var orderIDPattern = regexp.MustCompile(`^EFF-\d{6}$`)

// ValidOrderID reports whether s is a well-formed order id (^EFF-\d{6}$). Used
// by the tests here and, later, by the repository to reject a malformed id
// before it reaches Mongo.
func ValidOrderID(s string) bool { return orderIDPattern.MatchString(s) }

// NewOrderID returns a fresh customer-facing order id such as "EFF-483413".
//
// The six digits are drawn uniformly from 000000–999999 with crypto/rand, so
// there is no modulo bias and every value — including those with leading zeros
// — is equally likely. It returns an error rather than panicking if the system
// entropy source fails, and holds no state: two calls are independent, so it is
// safe to call repeatedly inside a collision-retry loop (schema.md §orders).
func NewOrderID() (string, error) {
	return newOrderID(rand.Reader)
}

// newOrderID is NewOrderID with an injectable entropy source, so the failure
// path can be exercised in tests with a reader that errors.
func newOrderID(r io.Reader) (string, error) {
	n, err := rand.Int(r, orderIDSpace)
	if err != nil {
		return "", fmt.Errorf("order: generate id: %w", err)
	}
	return "EFF-" + pad6(n.Int64()), nil
}

// pad6 renders n as exactly six digits with leading zeros, so the width is
// fixed and the pattern always matches. n is assumed in [0, 999999].
func pad6(n int64) string {
	const digits = "0123456789"
	out := make([]byte, OrderIDDigits)
	for i := OrderIDDigits - 1; i >= 0; i-- {
		out[i] = digits[n%10]
		n /= 10
	}
	return string(out)
}
