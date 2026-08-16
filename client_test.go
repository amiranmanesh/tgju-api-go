package tgju_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tgju "github.com/amiranmanesh/tgju-api-go"
	"github.com/amiranmanesh/tgju-api-go/internal/fixture"
)

func TestFetchParsesEveryBoard(t *testing.T) {
	t.Parallel()

	srv := fixture.Server(t)
	client := tgju.New(tgju.WithBaseURL(srv.URL), tgju.WithCacheTTL(0))

	tests := []struct {
		market     tgju.Market
		categories int
		key        string
		title      string
	}{
		{tgju.Currency, 2, "price_dollar_rl", "دلار"},
		{tgju.Gold, 5, "geram18", "طلای 18 عیار / 750"},
		{tgju.Coin, 5, "sekee", "سکه امامی"},
	}

	for _, tc := range tests {
		t.Run(string(tc.market), func(t *testing.T) {
			t.Parallel()

			snap, err := client.Fetch(t.Context(), tc.market)
			if err != nil {
				t.Fatalf("Fetch: %v", err)
			}
			if snap.Market != tc.market {
				t.Errorf("market = %q, want %q", snap.Market, tc.market)
			}
			if len(snap.Categories) != tc.categories {
				t.Errorf("got %d categories, want %d", len(snap.Categories), tc.categories)
			}
			if snap.Len() == 0 {
				t.Fatal("snapshot is empty")
			}
			if snap.FetchedAt.IsZero() {
				t.Error("FetchedAt is zero")
			}
			if !strings.HasSuffix(snap.Source, tc.market.Path()) {
				t.Errorf("source = %q, want it to end in %q", snap.Source, tc.market.Path())
			}

			item, ok := snap.Lookup(tc.key)
			if !ok {
				t.Fatalf("%q not found in %v", tc.key, snap.Keys())
			}
			if item.Title != tc.title {
				t.Errorf("title = %q, want %q", item.Title, tc.title)
			}
			if item.Market != tc.market {
				t.Errorf("item market = %q, want %q", item.Market, tc.market)
			}
			if item.Category == "" {
				t.Error("item has no category")
			}
			if item.Price.Value <= 0 {
				t.Errorf("price = %v, want a positive number", item.Price.Value)
			}
		})
	}
}

func TestFetchMapsARowCompletely(t *testing.T) {
	t.Parallel()

	srv := fixture.Server(t)
	client := tgju.New(tgju.WithBaseURL(srv.URL), tgju.WithCacheTTL(0))

	snap, err := client.Currency(t.Context())
	if err != nil {
		t.Fatalf("Currency: %v", err)
	}
	item, ok := snap.Lookup("price_dollar_rl")
	if !ok {
		t.Fatal("price_dollar_rl not found")
	}

	want := tgju.Item{
		Key:      "price_dollar_rl",
		Title:    "دلار",
		Market:   tgju.Currency,
		Category: "عنوان",
		Price:    tgju.Amount{Text: "1,864,000", Value: 1_864_000},
		Low:      tgju.Amount{Text: "1,860,800", Value: 1_860_800},
		High:     tgju.Amount{Text: "1,869,100", Value: 1_869_100},
		Change: tgju.Change{
			Status:  tgju.StatusLow,
			Percent: 0.32,
			Amount:  tgju.Amount{Text: "6,050", Value: 6050},
		},
		Time:       "11:49:45",
		ProfileURL: srv.URL + "/profile/price_dollar_rl",
	}
	if item != want {
		t.Errorf("item =\n %+v\nwant\n %+v", item, want)
	}

	if got := item.Price.Toman(); got != 186_400 {
		t.Errorf("Toman() = %v, want 186400", got)
	}
	if got := item.Change.Signum(); got != -1 {
		t.Errorf("Signum() = %d, want -1", got)
	}
	if got := item.Spread(); got != 8_300 {
		t.Errorf("Spread() = %v, want 8300", got)
	}
}

func TestFetchRejectsUnknownMarkets(t *testing.T) {
	t.Parallel()

	client := tgju.New(tgju.WithBaseURL("http://127.0.0.1:1"))

	_, err := client.Fetch(t.Context(), tgju.Market("crypto"))
	if !errors.Is(err, tgju.ErrUnknownMarket) {
		t.Fatalf("err = %v, want ErrUnknownMarket", err)
	}

	var tgjuErr *tgju.Error
	if !errors.As(err, &tgjuErr) {
		t.Fatalf("err is not a *tgju.Error: %v", err)
	}
	if tgjuErr.Temporary() {
		t.Error("an unknown market must not be reported as temporary")
	}
}

func TestFetchRetriesServerErrors(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64
	srv := fixture.ServerFunc(t, func(w http.ResponseWriter, _ *http.Request) bool {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return false
		}
		return true
	})

	client := tgju.New(
		tgju.WithBaseURL(srv.URL),
		tgju.WithCacheTTL(0),
		tgju.WithRetry(tgju.RetryPolicy{MaxAttempts: 3, Backoff: time.Millisecond}),
	)

	snap, err := client.Currency(t.Context())
	if err != nil {
		t.Fatalf("Currency: %v", err)
	}
	if snap.IsEmpty() {
		t.Fatal("snapshot is empty")
	}
	if got := calls.Load(); got != 3 {
		t.Errorf("upstream was called %d times, want 3", got)
	}
}

func TestFetchGivesUpAfterTheLastAttempt(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64
	srv := fixture.ServerFunc(t, func(w http.ResponseWriter, _ *http.Request) bool {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		return false
	})

	client := tgju.New(
		tgju.WithBaseURL(srv.URL),
		tgju.WithCacheTTL(0),
		tgju.WithRetry(tgju.RetryPolicy{MaxAttempts: 2, Backoff: time.Millisecond}),
	)

	_, err := client.Currency(t.Context())
	if !errors.Is(err, tgju.ErrUnexpectedStatus) {
		t.Fatalf("err = %v, want ErrUnexpectedStatus", err)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("upstream was called %d times, want 2", got)
	}

	var tgjuErr *tgju.Error
	if !errors.As(err, &tgjuErr) {
		t.Fatalf("err is not a *tgju.Error: %v", err)
	}
	switch {
	case tgjuErr.StatusCode != http.StatusInternalServerError:
		t.Errorf("StatusCode = %d, want 500", tgjuErr.StatusCode)
	case tgjuErr.Attempts != 2:
		t.Errorf("Attempts = %d, want 2", tgjuErr.Attempts)
	case !tgjuErr.Temporary():
		t.Error("a 500 must be reported as temporary")
	case tgjuErr.Market != tgju.Currency:
		t.Errorf("Market = %q, want currency", tgjuErr.Market)
	}
}

func TestFetchDoesNotRetryClientErrors(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64
	srv := fixture.ServerFunc(t, func(w http.ResponseWriter, _ *http.Request) bool {
		calls.Add(1)
		w.WriteHeader(http.StatusForbidden)
		return false
	})

	client := tgju.New(
		tgju.WithBaseURL(srv.URL),
		tgju.WithCacheTTL(0),
		tgju.WithRetry(tgju.RetryPolicy{MaxAttempts: 5, Backoff: time.Millisecond}),
	)

	_, err := client.Currency(t.Context())
	if !errors.Is(err, tgju.ErrUnexpectedStatus) {
		t.Fatalf("err = %v, want ErrUnexpectedStatus", err)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("upstream was called %d times, want 1", got)
	}

	var tgjuErr *tgju.Error
	if errors.As(err, &tgjuErr) && tgjuErr.Temporary() {
		t.Error("a 403 must not be reported as temporary")
	}
}

func TestFetchReportsUnparseablePages(t *testing.T) {
	t.Parallel()

	srv := fixture.ServerFunc(t, func(w http.ResponseWriter, _ *http.Request) bool {
		fmt.Fprint(w, "<html><body><h1>سایت در دست تعمیر است</h1></body></html>")
		return false
	})

	client := tgju.New(tgju.WithBaseURL(srv.URL), tgju.WithCacheTTL(0))

	_, err := client.Currency(t.Context())
	if !errors.Is(err, tgju.ErrParse) {
		t.Fatalf("err = %v, want ErrParse", err)
	}

	var tgjuErr *tgju.Error
	if errors.As(err, &tgjuErr) && tgjuErr.Temporary() {
		t.Error("a parse failure must not be reported as temporary")
	}
}

func TestFetchReportsEmptyBoards(t *testing.T) {
	t.Parallel()

	srv := fixture.ServerFunc(t, func(w http.ResponseWriter, _ *http.Request) bool {
		fmt.Fprint(w, `<table class="data-table market-table"><thead><tr><th>عنوان</th></tr></thead><tbody></tbody></table>`)
		return false
	})

	client := tgju.New(tgju.WithBaseURL(srv.URL), tgju.WithCacheTTL(0))

	if _, err := client.Currency(t.Context()); !errors.Is(err, tgju.ErrEmpty) {
		t.Fatalf("err = %v, want ErrEmpty", err)
	}
}

func TestFetchEnforcesTheBodyLimit(t *testing.T) {
	t.Parallel()

	srv := fixture.Server(t)
	client := tgju.New(tgju.WithBaseURL(srv.URL), tgju.WithCacheTTL(0), tgju.WithMaxBodyBytes(512))

	_, err := client.Currency(t.Context())
	if !errors.Is(err, tgju.ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

func TestFetchSendsTheConfiguredHeaders(t *testing.T) {
	t.Parallel()

	var got http.Header
	srv := fixture.ServerFunc(t, func(_ http.ResponseWriter, r *http.Request) bool {
		got = r.Header.Clone()
		return true
	})

	client := tgju.New(
		tgju.WithBaseURL(srv.URL),
		tgju.WithCacheTTL(0),
		tgju.WithUserAgent("acme-pricing/2.1"),
		tgju.WithHeader("X-Trace-Id", "abc123"),
	)

	if _, err := client.Currency(t.Context()); err != nil {
		t.Fatalf("Currency: %v", err)
	}
	if ua := got.Get("User-Agent"); ua != "acme-pricing/2.1" {
		t.Errorf("User-Agent = %q, want acme-pricing/2.1", ua)
	}
	if trace := got.Get("X-Trace-Id"); trace != "abc123" {
		t.Errorf("X-Trace-Id = %q, want abc123", trace)
	}
}

func TestFetchHonoursContextCancellation(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	srv := fixture.ServerFunc(t, func(_ http.ResponseWriter, r *http.Request) bool {
		select {
		case <-release:
		case <-r.Context().Done():
		}
		return false
	})
	t.Cleanup(func() { close(release) })

	client := tgju.New(tgju.WithBaseURL(srv.URL), tgju.WithCacheTTL(0))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := client.Currency(ctx); err == nil {
		t.Fatal("want an error from a cancelled context")
	}
}

func TestFetchAll(t *testing.T) {
	t.Parallel()

	srv := fixture.Server(t)
	client := tgju.New(tgju.WithBaseURL(srv.URL), tgju.WithCacheTTL(0))

	all, err := client.FetchAll(t.Context())
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if len(all) != len(tgju.Markets()) {
		t.Fatalf("got %d snapshots, want %d", len(all), len(tgju.Markets()))
	}
	for _, m := range tgju.Markets() {
		if all[m].IsEmpty() {
			t.Errorf("%s snapshot is empty", m)
		}
	}
}

func TestFetchAllFailsAsAWhole(t *testing.T) {
	t.Parallel()

	srv := fixture.ServerFunc(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/coin" {
			w.WriteHeader(http.StatusInternalServerError)
			return false
		}
		return true
	})

	client := tgju.New(
		tgju.WithBaseURL(srv.URL),
		tgju.WithCacheTTL(0),
		tgju.WithRetry(tgju.RetryPolicy{}),
	)

	if _, err := client.FetchAll(t.Context()); !errors.Is(err, tgju.ErrUnexpectedStatus) {
		t.Fatalf("err = %v, want ErrUnexpectedStatus", err)
	}
}

func TestItem(t *testing.T) {
	t.Parallel()

	srv := fixture.Server(t)
	client := tgju.New(tgju.WithBaseURL(srv.URL))

	tests := []struct {
		key    string
		market tgju.Market
	}{
		{"price_dollar_rl", tgju.Currency},
		{"geram18", tgju.Gold},
		{"sekee", tgju.Coin},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			item, err := client.Item(t.Context(), tc.key)
			if err != nil {
				t.Fatalf("Item: %v", err)
			}
			if item.Market != tc.market {
				t.Errorf("market = %q, want %q", item.Market, tc.market)
			}
		})
	}

	t.Run("unknown key", func(t *testing.T) {
		_, err := client.Item(t.Context(), "no_such_instrument")
		if !errors.Is(err, tgju.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("empty key", func(t *testing.T) {
		_, err := client.Item(t.Context(), "")
		if !errors.Is(err, tgju.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("restricted to one market", func(t *testing.T) {
		_, err := client.Item(t.Context(), "geram18", tgju.Currency)
		if !errors.Is(err, tgju.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})
}

func TestCacheServesRepeatedFetches(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64
	srv := fixture.ServerFunc(t, func(_ http.ResponseWriter, _ *http.Request) bool {
		calls.Add(1)
		return true
	})

	clock := newTestClock(time.Unix(1_700_000_000, 0))
	client := tgju.New(
		tgju.WithBaseURL(srv.URL),
		tgju.WithCacheTTL(time.Minute),
		tgju.WithClock(clock.Now),
	)

	for range 5 {
		if _, err := client.Currency(t.Context()); err != nil {
			t.Fatalf("Currency: %v", err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("upstream was called %d times, want 1", got)
	}

	clock.Advance(2 * time.Minute)
	if _, err := client.Currency(t.Context()); err != nil {
		t.Fatalf("Currency after expiry: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("upstream was called %d times after expiry, want 2", got)
	}
}

func TestInvalidateDropsTheCache(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64
	srv := fixture.ServerFunc(t, func(_ http.ResponseWriter, _ *http.Request) bool {
		calls.Add(1)
		return true
	})
	client := tgju.New(tgju.WithBaseURL(srv.URL), tgju.WithCacheTTL(time.Hour))

	if _, err := client.Currency(t.Context()); err != nil {
		t.Fatalf("Currency: %v", err)
	}
	client.Invalidate(tgju.Currency)
	if _, err := client.Currency(t.Context()); err != nil {
		t.Fatalf("Currency: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("upstream was called %d times, want 2", got)
	}

	client.Invalidate()
	if _, err := client.Currency(t.Context()); err != nil {
		t.Fatalf("Currency: %v", err)
	}
	if got := calls.Load(); got != 3 {
		t.Errorf("upstream was called %d times, want 3", got)
	}
}

// TestCacheCollapsesConcurrentMisses is the property that makes this client
// usable behind an HTTP API: a thundering herd on a cold cache must become one
// request to tgju.org, not one per caller.
func TestCacheCollapsesConcurrentMisses(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64
	gate := make(chan struct{})
	srv := fixture.ServerFunc(t, func(_ http.ResponseWriter, _ *http.Request) bool {
		calls.Add(1)
		<-gate // hold the first request open so the others pile up behind it
		return true
	})

	client := tgju.New(tgju.WithBaseURL(srv.URL), tgju.WithCacheTTL(time.Hour))

	const callers = 32
	var (
		wg   sync.WaitGroup
		errs = make([]error, callers)
	)
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = client.Currency(t.Context())
		}()
	}

	// Give the goroutines a moment to queue up, then let the fetch finish.
	time.Sleep(50 * time.Millisecond)
	close(gate)
	wg.Wait()

	if err := errors.Join(errs...); err != nil {
		t.Fatalf("concurrent fetches: %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("upstream was called %d times, want 1", got)
	}
}

func TestClientAccessors(t *testing.T) {
	t.Parallel()

	client := tgju.New(tgju.WithBaseURL("https://mirror.example/"), tgju.WithCacheTTL(90*time.Second))
	if got := client.BaseURL(); got != "https://mirror.example" {
		t.Errorf("BaseURL() = %q, want the trailing slash trimmed", got)
	}
	if got := client.CacheTTL(); got != 90*time.Second {
		t.Errorf("CacheTTL() = %v, want 90s", got)
	}
}

// testClock is a manually advanced clock, so cache expiry can be tested without
// sleeping.
type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func newTestClock(start time.Time) *testClock { return &testClock{t: start} }

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}
