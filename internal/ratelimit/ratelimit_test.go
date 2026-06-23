package ratelimit

import (
	"sync"
	"testing"
	"time"
)

// fakeClock is a manually-advanced clock so tests never sleep.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newTestLimiter(rps int) (*Limiter, *fakeClock) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	return newWithClock(rps, clk.now), clk
}

func allow(t *testing.T, l *Limiter, reason string) {
	t.Helper()
	if !l.Allow() {
		t.Fatalf("Allow() = false, expect true: %s", reason)
	}
}

func allowNTimes(t *testing.T, l *Limiter, n int, reason string) {
	t.Helper()
	for i := 1; i <= n; i++ {
		if !l.Allow() {
			t.Fatalf("Allow() call %d/%d = false, expect true: %s", i, n, reason)
		}
	}
}

func deny(t *testing.T, l *Limiter, reason string) {
	t.Helper()
	if l.Allow() {
		t.Fatalf("Allow() = true, expect false: %s", reason)
	}
}

func TestAllowsOneSecondBurstThenDenies(t *testing.T) {
	l, _ := newTestLimiter(3)

	allowNTimes(t, l, 3, "initial one-second burst")
	deny(t, l, "burst is exhausted")
}

func TestRefillsProportionallyOverTime(t *testing.T) {
	l, clk := newTestLimiter(2)

	allowNTimes(t, l, 2, "initial burst at 2 rps")
	deny(t, l, "initial burst is exhausted")

	clk.advance(500 * time.Millisecond)
	allow(t, l, "one token refills after half a second at 2 rps")
	deny(t, l, "only one token should have refilled")
}

func TestRefillCappedAtBurst(t *testing.T) {
	l, clk := newTestLimiter(2)

	clk.advance(10 * time.Second) // would add 20 tokens, but burst is 2
	allowNTimes(t, l, 2, "bucket refills only to burst size")
	deny(t, l, "bucket must not exceed its burst")
}

func TestConcurrentAllowIsRaceFree(t *testing.T) {
	// Run with -race: exercises the mutex under contention.
	l := New(1000)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.Allow()
		}()
	}
	wg.Wait()
}
