package admin_test

import (
	"testing"
	"time"

	"github.com/enerzia/enerzia-be/internal/admin"
)

func TestLimiterAllowsNewIP(t *testing.T) {
	l := admin.NewLimiter()
	if !l.Allow("1.2.3.4") {
		t.Error("Allow() = false for a fresh IP, want true")
	}
}

func TestLimiterBlocksAfterLimit(t *testing.T) {
	l := admin.NewLimiter()
	const ip = "1.2.3.4"

	// Record exactly 10 failures.
	for i := range 10 {
		_ = i
		l.RecordFailure(ip)
	}

	if l.Allow(ip) {
		t.Error("Allow() = true after 10 failures, want false")
	}
}

func TestLimiterDoesNotBlockBeforeLimit(t *testing.T) {
	l := admin.NewLimiter()
	const ip = "1.2.3.4"

	// 9 failures — one short of the limit.
	for i := range 9 {
		_ = i
		l.RecordFailure(ip)
	}

	if !l.Allow(ip) {
		t.Error("Allow() = false after 9 failures, want true (limit is 10)")
	}
}

func TestLimiterIsolatesIPs(t *testing.T) {
	l := admin.NewLimiter()
	const ipA = "1.2.3.4"
	const ipB = "5.6.7.8"

	// Exhaust ipA.
	for i := range 10 {
		_ = i
		l.RecordFailure(ipA)
	}

	// ipB must be unaffected.
	if !l.Allow(ipB) {
		t.Error("Allow(ipB) = false after ipA was exhausted; IPs must be isolated")
	}
}

func TestLimiterPrunesExpiredEntries(t *testing.T) {
	// Use a clock we can advance manually.
	var now time.Time
	now = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	l := admin.NewLimiterAt(func() time.Time { return now })
	const ip = "1.2.3.4"

	// Record 10 failures at t=0.
	for i := range 10 {
		_ = i
		l.RecordFailure(ip)
	}

	// Must be blocked.
	if l.Allow(ip) {
		t.Fatal("Allow() = true right after 10 failures, want false")
	}

	// Advance time beyond the 15-minute window.
	now = now.Add(16 * time.Minute)

	// Entries should be pruned on the next Allow call.
	if !l.Allow(ip) {
		t.Error("Allow() = false after window expired, want true (entries should be pruned)")
	}
}

func TestLimiterIsGoroutineSafe(t *testing.T) {
	l := admin.NewLimiter()
	const ip = "1.2.3.4"
	done := make(chan struct{})

	go func() {
		for range 20 {
			l.RecordFailure(ip)
		}
		close(done)
	}()

	for range 20 {
		l.Allow(ip)
	}
	<-done
}
