package services

import (
	"sync"
	"time"
)

// Abuse limits, and the constraint that shapes them.
//
// The usual answer is a bucket per client IP. LinkUp cannot do that: the whole
// product promises never to look at a visitor's address, and a privacy claim
// that bends the moment it becomes inconvenient is not a claim. So the limits
// are drawn on what is already known without profiling anyone:
//
//   - writes are limited per IDENTITY (an OIDC session or an API key), which we
//     know because the caller authenticated;
//   - PIN attempts are limited per LINK — the resource under attack — instead of
//     per visitor, who is by design unidentifiable. It protects the thing that
//     matters and needs nobody's address to do it;
//   - the public redirect has no per-visitor limit on purpose. It is the hot
//     path and it is the product; if it ever needs shielding, that belongs at
//     the edge, not here.

// TokenBucket is a fixed-rate bucket keyed by an arbitrary string.
type TokenBucket struct {
	mu       sync.Mutex
	capacity float64
	refill   float64 // tokens per second
	entries  map[string]*bucketEntry
}

type bucketEntry struct {
	tokens float64
	seen   time.Time
}

// NewTokenBucket builds a limiter allowing `capacity` operations in a burst,
// refilling to full over `window`.
func NewTokenBucket(capacity int, window time.Duration) *TokenBucket {
	if capacity < 1 {
		capacity = 1
	}
	if window <= 0 {
		window = time.Minute
	}
	return &TokenBucket{
		capacity: float64(capacity),
		refill:   float64(capacity) / window.Seconds(),
		entries:  make(map[string]*bucketEntry),
	}
}

// Allow consumes one token for key, reporting whether there was one to take.
func (b *TokenBucket) Allow(key string) bool {
	if key == "" {
		// No identity, no allowance. Callers must authenticate first.
		return false
	}
	now := time.Now()

	b.mu.Lock()
	defer b.mu.Unlock()

	entry, ok := b.entries[key]
	if !ok {
		b.entries[key] = &bucketEntry{tokens: b.capacity - 1, seen: now}
		b.sweepLocked(now)
		return true
	}

	entry.tokens += now.Sub(entry.seen).Seconds() * b.refill
	if entry.tokens > b.capacity {
		entry.tokens = b.capacity
	}
	entry.seen = now

	if entry.tokens < 1 {
		return false
	}
	entry.tokens--
	return true
}

// sweepLocked drops idle entries so the map cannot grow without bound. Called
// only while the mutex is held, and only on the cold path of a new key.
func (b *TokenBucket) sweepLocked(now time.Time) {
	if len(b.entries) < 1024 {
		return
	}
	full := time.Duration(b.capacity/b.refill) * time.Second
	for key, entry := range b.entries {
		if now.Sub(entry.seen) > full {
			delete(b.entries, key)
		}
	}
}

// PINGuard budgets PIN attempts per link.
type PINGuard struct {
	mu       sync.Mutex
	maxTries int
	lockFor  time.Duration
	state    map[string]*pinState
}

type pinState struct {
	failures int
	until    time.Time
	seen     time.Time
}

// NewPINGuard allows maxTries wrong PINs before locking that link for lockFor.
func NewPINGuard(maxTries int, lockFor time.Duration) *PINGuard {
	if maxTries < 1 {
		maxTries = 5
	}
	if lockFor <= 0 {
		lockFor = 15 * time.Minute
	}
	return &PINGuard{maxTries: maxTries, lockFor: lockFor, state: make(map[string]*pinState)}
}

// Locked reports whether this link is currently refusing attempts, and for how
// much longer.
func (g *PINGuard) Locked(linkID string) (bool, time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()

	entry, ok := g.state[linkID]
	if !ok {
		return false, 0
	}
	if remaining := time.Until(entry.until); remaining > 0 {
		return true, remaining
	}
	return false, 0
}

// Failed records a wrong PIN and locks the link once the budget is spent. The
// wait grows with each lockout, so a patient attacker gets slower, not luckier.
func (g *PINGuard) Failed(linkID string) {
	now := time.Now()

	g.mu.Lock()
	defer g.mu.Unlock()

	entry, ok := g.state[linkID]
	if !ok {
		entry = &pinState{}
		g.state[linkID] = entry
		g.sweepLocked(now)
	}
	entry.failures++
	entry.seen = now

	if entry.failures >= g.maxTries {
		rounds := entry.failures / g.maxTries
		wait := g.lockFor * time.Duration(rounds)
		if max := 6 * time.Hour; wait > max {
			wait = max
		}
		entry.until = now.Add(wait)
	}
}

// Succeeded clears the record for a link after a correct PIN.
func (g *PINGuard) Succeeded(linkID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.state, linkID)
}

func (g *PINGuard) sweepLocked(now time.Time) {
	if len(g.state) < 1024 {
		return
	}
	for key, entry := range g.state {
		if now.After(entry.until) && now.Sub(entry.seen) > 24*time.Hour {
			delete(g.state, key)
		}
	}
}
