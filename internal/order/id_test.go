package order

import (
	"errors"
	"strings"
	"testing"
	"testing/iotest"
)

func TestValidOrderID(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"EFF-483413", true},
		{"EFF-000000", true}, // leading zeros are admitted
		{"EFF-999999", true},
		{"EFF-48341", false},   // five digits
		{"EFF-4834133", false}, // seven digits
		{"eff-483413", false},  // lowercase prefix
		{"EFF483413", false},   // missing hyphen
		{"EFF-48341a", false},  // non-digit
		{"XFF-483413", false},  // wrong prefix
		{" EFF-483413", false}, // leading space
		{"EFF-483413 ", false}, // trailing space
		{"EFF-48 413", false},  // embedded space
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := ValidOrderID(tt.in); got != tt.want {
				t.Errorf("ValidOrderID(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestNewOrderIDFormatAndValidates(t *testing.T) {
	for range 2000 {
		id, err := NewOrderID()
		if err != nil {
			t.Fatalf("NewOrderID() error = %v", err)
		}
		if !ValidOrderID(id) {
			t.Fatalf("NewOrderID() = %q, which does not match EFF-######", id)
		}
		if !strings.HasPrefix(id, "EFF-") || len(id) != len("EFF-")+OrderIDDigits {
			t.Fatalf("NewOrderID() = %q, malformed prefix or width", id)
		}
	}
}

func TestNewOrderIDVariesAndIsIndependent(t *testing.T) {
	// Two calls share no state, so a run of draws must not collapse onto a
	// handful of values.
	seen := make(map[string]bool)
	for range 500 {
		id, err := NewOrderID()
		if err != nil {
			t.Fatalf("NewOrderID() error = %v", err)
		}
		seen[id] = true
	}
	if len(seen) < 450 {
		t.Errorf("only %d distinct ids in 500 draws; generator looks weak", len(seen))
	}
}

func TestNewOrderIDIsUniformOverTheFullSpace(t *testing.T) {
	// The numeric part is drawn from exactly 1,000,000 values, so there is no
	// modulo bias and every leading digit — including 0 — must appear, with the
	// range spanning both ends. This pins the "uniform over 000000–999999"
	// choice: a biased or truncated generator would miss a bucket or an end.
	if orderIDSpace.Int64() != 1_000_000 {
		t.Fatalf("orderIDSpace = %d, want 1_000_000", orderIDSpace.Int64())
	}
	var buckets [10]int
	sawLow, sawHigh := false, false // sawLow ⇒ a leading-zero id exists
	for range 20000 {
		id, err := NewOrderID()
		if err != nil {
			t.Fatalf("NewOrderID() error = %v", err)
		}
		digits := strings.TrimPrefix(id, "EFF-")
		buckets[digits[0]-'0']++
		if digits < "100000" {
			sawLow = true
		}
		if digits >= "900000" {
			sawHigh = true
		}
	}
	for d, n := range buckets {
		if n == 0 {
			t.Errorf("leading digit %d never appeared in 20000 draws; distribution is not uniform", d)
		}
	}
	if !sawLow {
		t.Error("no id below 100000 in 20000 draws; leading zeros are being suppressed")
	}
	if !sawHigh {
		t.Error("no id at or above 900000 in 20000 draws; the top of the range is unreachable")
	}
}

func TestNewOrderIDErrorsOnBadEntropy(t *testing.T) {
	// A failed entropy source must surface as an error, not a panic and not a
	// predictable id, so a collision-retry loop can decide what to do.
	_, err := newOrderID(iotest.ErrReader(errors.New("entropy exhausted")))
	if err == nil {
		t.Fatal("newOrderID() with a failing reader returned no error")
	}
	if !strings.Contains(err.Error(), "generate id") {
		t.Errorf("error = %v, want it wrapped with context", err)
	}
}

func TestPad6(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{0, "000000"},
		{7, "000007"},
		{4213, "004213"},
		{483413, "483413"},
		{999999, "999999"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := pad6(tt.in); got != tt.want {
				t.Errorf("pad6(%d) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
