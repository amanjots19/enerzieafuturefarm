package admin

import (
	"sync"
	"time"
)

// Per-IP failure thresholds. 10 failures in 15 minutes triggers a block.
// These are deliberately conservative — the admin login is not a high-volume
// endpoint and the only legitimate user is one person.
const (
	defaultFailureLimit = 10
	defaultRateWindow   = 15 * time.Minute
)

// Limiter tracks per-IP failed login attempts and enforces a sliding-window
// block when a single IP exceeds the allowed failure count.
//
// Client IP is taken from r.RemoteAddr ONLY — not X-Forwarded-For.
// X-Forwarded-For is attacker-controlled: there is no trusted proxy in front
// of this service today, so honouring it would let a caller bypass the limit
// by rotating headers. If a reverse proxy is ever placed in front, the IP
// should come from the proxy's own header after the proxy is verified and the
// code changed here deliberately.
type Limiter struct {
	mu      sync.Mutex
	entries map[string][]time.Time
	limit   int
	window  time.Duration
	now     func() time.Time
}

// NewLimiter returns a Limiter with production defaults.
func NewLimiter() *Limiter {
	return NewLimiterAt(time.Now)
}

// NewLimiterAt is NewLimiter with an injectable clock for deterministic tests.
func NewLimiterAt(now func() time.Time) *Limiter {
	return &Limiter{
		entries: make(map[string][]time.Time),
		limit:   defaultFailureLimit,
		window:  defaultRateWindow,
		now:     now,
	}
}

// Allow reports whether ip may attempt a login. It returns false when the IP
// has already reached the failure limit within the sliding window. It also
// prunes stale entries to prevent unbounded map growth.
func (l *Limiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prune(ip)
	return len(l.entries[ip]) < l.limit
}

// RecordFailure records a failed login attempt for ip.
func (l *Limiter) RecordFailure(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries[ip] = append(l.entries[ip], l.now())
}

// prune removes timestamps outside the current window for ip. Must be called
// with l.mu held.
func (l *Limiter) prune(ip string) {
	cutoff := l.now().Add(-l.window)
	times := l.entries[ip]
	i := 0
	for i < len(times) && times[i].Before(cutoff) {
		i++
	}
	if i == 0 {
		return
	}
	if i == len(times) {
		delete(l.entries, ip)
	} else {
		l.entries[ip] = times[i:]
	}
}
