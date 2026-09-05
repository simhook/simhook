// Package ratelimit is a keyed token bucket: one limiter per key, created on
// first use and forgotten when idle. Keys are whatever the caller throttles
// by: a client address, an account email.
package ratelimit

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	maxEntries = 10000
	idleFor    = 10 * time.Minute
)

// Keyed holds one token bucket per key.
type Keyed struct {
	mu      sync.Mutex
	entries map[string]*entry
	rate    rate.Limit
	burst   int
}

type entry struct {
	l    *rate.Limiter
	seen time.Time
}

// NewKeyed allows burst calls per key at once, then r per second.
func NewKeyed(r rate.Limit, burst int) *Keyed {
	return &Keyed{entries: map[string]*entry{}, rate: r, burst: burst}
}

// Allow reports whether key may proceed now, consuming a token when it may.
func (k *Keyed) Allow(key string) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	now := time.Now()
	if len(k.entries) > maxEntries {
		for id, e := range k.entries {
			if now.Sub(e.seen) > idleFor {
				delete(k.entries, id)
			}
		}
	}
	e, ok := k.entries[key]
	if !ok {
		e = &entry{l: rate.NewLimiter(k.rate, k.burst)}
		k.entries[key] = e
	}
	e.seen = now
	return e.l.Allow()
}
