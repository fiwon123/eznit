// Package ratelimit is a small, in-memory fixed-window limiter for the
// authentication routes.
//
// It is per process: the API and the GUI each enforce their own limits. That is
// enough for a self-hosted tool and avoids a shared store (Redis, a table) and
// the per-request write that would come with it.
package ratelimit

import (
	"sync"
	"time"
)

type entry struct {
	count   int
	resetAt time.Time
}

// Limiter counts requests per key in a fixed window.
type Limiter struct {
	mu        sync.Mutex
	window    time.Duration
	now       func() time.Time
	entries   map[string]*entry
	lastSweep time.Time
}

// New returns a Limiter whose window is the given duration.
func New(window time.Duration) *Limiter {
	return NewWithClock(window, time.Now)
}

// NewWithClock is New with an injectable clock, so tests can advance time.
func NewWithClock(window time.Duration, now func() time.Time) *Limiter {
	if window <= 0 {
		window = time.Minute
	}
	n := now()
	return &Limiter{
		window:    window,
		now:       now,
		entries:   make(map[string]*entry),
		lastSweep: n,
	}
}

// Allow records one request for key and reports whether it is within limit.
//
// A non-positive limit means "no limit" for that key. When the request is over
// the limit, it also returns how long until the window resets, so the caller
// can set Retry-After.
func (l *Limiter) Allow(key string, limit int) (bool, time.Duration) {
	if limit <= 0 {
		return true, 0
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweepLocked(now)

	e, ok := l.entries[key]
	if !ok || !now.Before(e.resetAt) {
		e = &entry{resetAt: now.Add(l.window)}
		l.entries[key] = e
	}

	if e.count >= limit {
		return false, e.resetAt.Sub(now)
	}
	e.count++
	return true, 0
}

// sweepLocked drops expired entries at most once per window. There is no
// background goroutine to start or stop: the map is bounded by the keys seen in
// roughly one window.
func (l *Limiter) sweepLocked(now time.Time) {
	if now.Sub(l.lastSweep) < l.window {
		return
	}
	for k, e := range l.entries {
		if !now.Before(e.resetAt) {
			delete(l.entries, k)
		}
	}
	l.lastSweep = now
}
