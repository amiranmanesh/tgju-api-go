package tgju

import (
	"context"
	"sync"
	"time"
)

// cache memoises one snapshot per market for a fixed window and collapses the
// concurrent fetches of the same market into a single outgoing request.
//
// The two behaviours belong together: without collapsing, a cold cache under
// load sends one request to tgju per caller, which is exactly the burst the
// cache was meant to prevent.
type cache struct {
	ttl time.Duration
	now func() time.Time

	mu      sync.Mutex
	entries map[Market]*entry
}

// entry is either in flight — ready is open, and the goroutine that created it
// is fetching — or settled, with snap and err final until expiry.
type entry struct {
	ready   chan struct{}
	snap    Snapshot
	err     error
	expires time.Time
}

func newCache(ttl time.Duration, now func() time.Time) *cache {
	return &cache{ttl: ttl, now: now, entries: map[Market]*entry{}}
}

// fetcher produces a fresh snapshot. It is called at most once per cache miss.
type fetcher func(context.Context) (Snapshot, error)

// do returns the cached snapshot for m, fetching it when the cache is cold or
// stale. The second result reports whether the snapshot came from memory.
//
// When several goroutines miss at once, the first runs fetch and the rest wait
// for its result. Waiting honours the caller's context, so a client that gives
// up does not block on someone else's slow request; the fetch itself keeps
// running so that whoever is still waiting gets an answer.
func (c *cache) do(ctx context.Context, m Market, fetch fetcher) (Snapshot, bool, error) {
	if c == nil || c.ttl <= 0 {
		snap, err := fetch(ctx)
		return snap, false, err
	}

	for {
		c.mu.Lock()
		e, ok := c.entries[m]
		switch {
		case ok && e.settled() && c.now().Before(e.expires):
			c.mu.Unlock()
			return e.snap, true, e.err
		case ok && !e.settled():
			// Someone else is already fetching this market.
			c.mu.Unlock()
			select {
			case <-e.ready:
				continue // re-read under the lock: it may have expired already
			case <-ctx.Done():
				return Snapshot{}, false, ctx.Err()
			}
		}

		leader := &entry{ready: make(chan struct{})}
		c.entries[m] = leader
		c.mu.Unlock()

		// The fetch runs detached from the caller's cancellation so that the
		// goroutine which happened to arrive first cannot, by walking away,
		// abort the request everyone else is waiting on. It stays bounded:
		// Client.fetch applies its own deadline.
		go func() {
			snap, err := fetch(context.WithoutCancel(ctx))

			c.mu.Lock()
			leader.snap, leader.err = snap, err
			leader.expires = c.now().Add(c.ttl)
			if err != nil {
				// A failure is not worth a full TTL of memory, but caching it
				// briefly stops a failing upstream from being hammered.
				leader.expires = c.now().Add(min(c.ttl, failureTTL))
			}
			c.mu.Unlock()
			close(leader.ready)
		}()

		select {
		case <-leader.ready:
			// Closing the channel publishes the two fields written above.
			return leader.snap, false, leader.err
		case <-ctx.Done():
			return Snapshot{}, false, ctx.Err()
		}
	}
}

// failureTTL caps how long a failed fetch is remembered.
const failureTTL = 2 * time.Second

// settled reports whether the entry's fetch has finished.
func (e *entry) settled() bool {
	select {
	case <-e.ready:
		return true
	default:
		return false
	}
}

// invalidate drops the cached snapshots for the given markets, or all of them
// when none is named. An in-flight fetch is left alone; only its result stops
// being reused.
func (c *cache) invalidate(markets ...Market) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(markets) == 0 {
		clear(c.entries)
		return
	}
	for _, m := range markets {
		delete(c.entries, m)
	}
}
