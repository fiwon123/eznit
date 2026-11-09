package ratelimit

import (
	"testing"
	"time"
)

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time          { return c.now }
func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func newFake(window time.Duration) (*Limiter, *fakeClock) {
	clock := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	return NewWithClock(window, clock.Now), clock
}

func TestAllowsUpToTheLimitThenRefuses(t *testing.T) {
	l, _ := newFake(time.Minute)

	for i := 1; i <= 3; i++ {
		if ok, _ := l.Allow("k", 3); !ok {
			t.Fatalf("request %d should be allowed", i)
		}
	}
	ok, retry := l.Allow("k", 3)
	if ok {
		t.Fatal("the fourth request should be refused")
	}
	if retry <= 0 || retry > time.Minute {
		t.Fatalf("retryAfter = %v, want within (0, 1m]", retry)
	}
}

func TestWindowResets(t *testing.T) {
	l, clock := newFake(time.Minute)

	for i := 0; i < 2; i++ {
		l.Allow("k", 2)
	}
	if ok, _ := l.Allow("k", 2); ok {
		t.Fatal("expected the limit to be reached")
	}

	clock.Advance(time.Minute)
	if ok, _ := l.Allow("k", 2); !ok {
		t.Fatal("the window should have reset after one window")
	}
}

func TestKeysAreIndependent(t *testing.T) {
	l, _ := newFake(time.Minute)

	if ok, _ := l.Allow("a", 1); !ok {
		t.Fatal("first key should be allowed")
	}
	if ok, _ := l.Allow("a", 1); ok {
		t.Fatal("second request for the same key should be refused")
	}
	if ok, _ := l.Allow("b", 1); !ok {
		t.Fatal("a different key must have its own counter")
	}
}

func TestNonPositiveLimitMeansNoLimit(t *testing.T) {
	l, _ := newFake(time.Minute)
	for i := 0; i < 100; i++ {
		if ok, _ := l.Allow("k", 0); !ok {
			t.Fatalf("a non-positive limit must allow, request %d was refused", i)
		}
	}
}

func TestRetryAfterShrinksAsTheWindowDrains(t *testing.T) {
	l, clock := newFake(time.Minute)
	l.Allow("k", 1)

	_, first := l.Allow("k", 1)
	clock.Advance(20 * time.Second)
	_, second := l.Allow("k", 1)
	if second >= first {
		t.Fatalf("retryAfter should shrink as time passes: first %v, second %v", first, second)
	}
}

func TestSweepDropsExpiredKeys(t *testing.T) {
	l, clock := newFake(time.Minute)
	l.Allow("old", 1)

	clock.Advance(2 * time.Minute)
	l.Allow("new", 1) // this Allow runs the sweep

	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.entries["old"]; ok {
		t.Fatal("the expired key should have been swept")
	}
	if _, ok := l.entries["new"]; !ok {
		t.Fatal("the live key must survive the sweep")
	}
}
