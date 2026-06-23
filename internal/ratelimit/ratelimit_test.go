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

func TestBurstThenDeny(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	l := newWithClock(3, clk.now)

	for i := 0; i < 3; i++ {
		if !l.Allow() {
			t.Fatalf("request %d should be allowed within burst", i)
		}
	}
	if l.Allow() {
		t.Fatal("4th request should be denied once burst is exhausted")
	}
}

func TestRefill(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	l := newWithClock(2, clk.now) // 2 rps

	if !l.Allow() || !l.Allow() {
		t.Fatal("first two should pass")
	}
	if l.Allow() {
		t.Fatal("third should be denied")
	}

	clk.advance(500 * time.Millisecond) // refills 1 token at 2/s
	if !l.Allow() {
		t.Fatal("should pass after refill")
	}
	if l.Allow() {
		t.Fatal("only one token refilled; should deny again")
	}
}

func TestRefillCappedAtMax(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	l := newWithClock(2, clk.now)

	clk.advance(10 * time.Second) // would add 20 tokens, but capacity is 2
	if !l.Allow() || !l.Allow() {
		t.Fatal("two tokens should be available")
	}
	if l.Allow() {
		t.Fatal("bucket must not exceed its capacity")
	}
}

func TestFractionalRateStillAdmits(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	l := newWithClock(0.5, clk.now) // half a request per second

	if !l.Allow() {
		t.Fatal("fractional rate must allow the initial burst token")
	}
	if l.Allow() {
		t.Fatal("second request should be denied until refill")
	}
	clk.advance(2 * time.Second) // 0.5 rps -> 1 token after 2s
	if !l.Allow() {
		t.Fatal("should pass after fractional refill")
	}
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
