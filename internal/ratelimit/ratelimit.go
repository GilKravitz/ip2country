// Package ratelimit implements a token-bucket rate limiter from scratch.
//
// The PRD forbids rate-limiting libraries (including golang.org/x/time/rate),
// so this is a deliberately small, self-contained token bucket: tokens refill
// continuously at a fixed rate and each allowed request consumes one. It is the
// classic algorithm — smooth average rate with a one-second burst allowance.
package ratelimit

import (
	"math"
	"sync"
	"time"
)

// Limiter is a thread-safe global token-bucket limiter.
type Limiter struct {
	mu     sync.Mutex
	tokens float64   // currently available tokens
	max    float64   // bucket capacity (burst)
	rate   float64   // tokens added per second
	last   time.Time // last time tokens were refilled
	now    func() time.Time
}

// New returns a Limiter allowing rps requests per second with a burst of rps.
func New(rps float64) *Limiter {
	return newWithClock(rps, time.Now)
}

// newWithClock allows tests to inject a deterministic clock.
func newWithClock(rps float64, now func() time.Time) *Limiter {
	// Burst is at least one token so fractional rates (e.g. 0.5 rps) can ever
	// admit a request; otherwise tokens never reach the 1 needed by Allow.
	max := math.Max(rps, 1)
	return &Limiter{
		tokens: max,
		max:    max,
		rate:   rps,
		last:   now(),
		now:    now,
	}
}

// Allow reports whether one request may proceed now, consuming a token if so.
func (l *Limiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	elapsed := now.Sub(l.last).Seconds()
	if elapsed > 0 {
		l.tokens = min(l.max, l.tokens+elapsed*l.rate)
		l.last = now
	}

	if l.tokens >= 1 {
		l.tokens--
		return true
	}
	return false
}
