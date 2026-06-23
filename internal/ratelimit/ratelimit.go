// Package ratelimit implements a token-bucket rate limiter from scratch.
//
// The PRD forbids rate-limiting libraries (including golang.org/x/time/rate),
// so this is a deliberately small, self-contained token bucket. RATE_LIMIT_RPS=N
// refills N tokens per second and saves at most N tokens, allowing up to one
// second of burst after idle time.
package ratelimit

import (
	"sync"
	"time"
)

// Limiter is a thread-safe global token-bucket limiter.
type Limiter struct {
	mu           sync.Mutex
	tokens       float64          // Current number of tokens in the bucket.
	burst        float64          // Maximum number of tokens the bucket can hold (one second of burst).
	refillRate   float64          // Number of tokens to add per second.
	lastRefillAt time.Time        // Last time the bucket was refilled.
	now          func() time.Time // Function to get the current time, allowing for testing with a fake clock.
}

// New returns a Limiter allowing rps requests per second with a one-second burst.
func New(rps int) *Limiter {
	return newWithClock(rps, time.Now)
}

// newWithClock allows tests to inject a deterministic clock.
func newWithClock(rps int, now func() time.Time) *Limiter {
	return &Limiter{
		tokens:       float64(rps),
		burst:        float64(rps),
		refillRate:   float64(rps),
		lastRefillAt: now(),
		now:          now,
	}
}

// Allow reports whether one request may proceed now, consuming one token if so.
func (l *Limiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	elapsedSeconds := now.Sub(l.lastRefillAt).Seconds()
	if elapsedSeconds > 0 {
		refilledTokens := elapsedSeconds * l.refillRate
		l.tokens = min(l.burst, l.tokens+refilledTokens) // Cap tokens at burst size.
		l.lastRefillAt = now
	}

	if l.tokens < 1 {
		return false
	}

	l.tokens--
	return true
}
