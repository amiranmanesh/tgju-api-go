package tgju

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/amiranmanesh/tgju-api-go/internal/scrape"
)

// Client reads price boards from tgju.org.
//
// A Client is safe for concurrent use and is meant to be built once and kept:
// it owns the HTTP connection pool and the snapshot cache, both of which are
// wasted when a client is created per request.
//
// The zero value is not usable; call [New].
type Client struct {
	cfg   config
	cache *cache
}

// New returns a client configured by opts.
//
//	client := tgju.New(
//	    tgju.WithTimeout(10*time.Second),
//	    tgju.WithCacheTTL(time.Minute),
//	)
func New(opts ...Option) *Client {
	cfg := newConfig(opts...)
	return &Client{cfg: cfg, cache: newCache(cfg.cacheTTL, cfg.now)}
}

// Fetch returns the current snapshot of a board, serving it from the cache when
// one was taken within the configured TTL.
//
// Concurrent calls for the same market while the cache is cold are collapsed
// into a single request to tgju.org.
func (c *Client) Fetch(ctx context.Context, m Market) (Snapshot, error) {
	if !m.Valid() {
		return Snapshot{}, &Error{Op: "fetch", Market: m, Err: fmt.Errorf("%w: %q", ErrUnknownMarket, m)}
	}

	snap, cached, err := c.cache.do(ctx, m, func(ctx context.Context) (Snapshot, error) {
		return c.load(ctx, m)
	})
	if err != nil {
		return Snapshot{}, err
	}
	if cached {
		c.cfg.logger.DebugContext(ctx, "tgju: served snapshot from cache",
			slog.String("market", string(m)), slog.Int("items", snap.Len()))
	}
	return snap, nil
}

// Currency returns the foreign exchange board.
func (c *Client) Currency(ctx context.Context) (Snapshot, error) { return c.Fetch(ctx, Currency) }

// Gold returns the gold, silver and mesghal board.
func (c *Client) Gold(ctx context.Context) (Snapshot, error) { return c.Fetch(ctx, Gold) }

// Coin returns the Bahar Azadi coin board.
func (c *Client) Coin(ctx context.Context) (Snapshot, error) { return c.Fetch(ctx, Coin) }

// FetchAll returns a snapshot per market, fetched concurrently. Passing no
// market fetches every supported one.
//
// It is all or nothing: the first failure is returned and the partial result is
// discarded, because a caller that wanted "whatever succeeded" can loop over
// [Fetch] itself and decide what a hole in the data means for it.
func (c *Client) FetchAll(ctx context.Context, markets ...Market) (map[Market]Snapshot, error) {
	if len(markets) == 0 {
		markets = Markets()
	}

	var (
		mu   sync.Mutex
		wg   sync.WaitGroup
		out  = make(map[Market]Snapshot, len(markets))
		errs = make([]error, len(markets))
	)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for i, m := range markets {
		wg.Add(1)
		go func() {
			defer wg.Done()

			snap, err := c.Fetch(ctx, m)
			if err != nil {
				errs[i] = err
				cancel() // the result is discarded anyway; stop the others
				return
			}
			mu.Lock()
			out[m] = snap
			mu.Unlock()
		}()
	}
	wg.Wait()

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return out, nil
}

// Item finds a single instrument by its tgju key — "price_dollar_rl",
// "geram18", "sekee" — across the given markets, or across all of them when
// none is named.
//
// It returns an error wrapping [ErrNotFound] when no board publishes the key.
// With the cache on this costs at most one fetch per market per TTL, so it is a
// reasonable call to make per HTTP request in a service.
func (c *Client) Item(ctx context.Context, key string, markets ...Market) (Item, error) {
	if key == "" {
		return Item{}, &Error{Op: "lookup", Err: fmt.Errorf("%w: empty key", ErrNotFound)}
	}
	if len(markets) == 0 {
		markets = Markets()
	}

	snapshots, err := c.FetchAll(ctx, markets...)
	if err != nil {
		return Item{}, err
	}
	// Markets() is sorted, so the answer does not depend on map iteration order.
	for _, m := range markets {
		if item, ok := snapshots[m].Lookup(key); ok {
			return item, nil
		}
	}
	return Item{}, &Error{Op: "lookup", Err: fmt.Errorf("%w: %q", ErrNotFound, key)}
}

// Invalidate drops cached snapshots for the given markets, or for all of them
// when none is named. The next fetch goes to tgju.org.
func (c *Client) Invalidate(markets ...Market) { c.cache.invalidate(markets...) }

// BaseURL returns the site the client reads from.
func (c *Client) BaseURL() string { return c.cfg.baseURL }

// CacheTTL returns how long snapshots are reused. Zero means the cache is off.
func (c *Client) CacheTTL() time.Duration { return c.cfg.cacheTTL }

// load performs the real work behind a cache miss: fetch the page, parse it,
// map it onto the domain model.
func (c *Client) load(ctx context.Context, m Market) (Snapshot, error) {
	url := c.cfg.baseURL + m.Path()

	ctx, cancel := context.WithTimeout(ctx, c.cfg.timeout)
	defer cancel()

	body, err := c.get(ctx, m, url)
	if err != nil {
		return Snapshot{}, err
	}

	tables, err := scrape.Tables(bytes.NewReader(body))
	if err != nil {
		return Snapshot{}, &Error{Op: "parse", Market: m, URL: url, Err: fmt.Errorf("%w: %w", ErrParse, err)}
	}

	snap := snapshotFrom(m, c.cfg.baseURL, url, c.cfg.now(), tables)
	if snap.IsEmpty() {
		return Snapshot{}, &Error{Op: "parse", Market: m, URL: url, Err: ErrEmpty}
	}

	c.cfg.logger.DebugContext(ctx, "tgju: parsed snapshot",
		slog.String("market", string(m)),
		slog.Int("categories", len(snap.Categories)),
		slog.Int("items", snap.Len()))

	return snap, nil
}
