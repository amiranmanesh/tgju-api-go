# Library guide

```bash
go get github.com/amiranmanesh/tgju-api-go
```

```go
import tgju "github.com/amiranmanesh/tgju-api-go"
```

The import path ends in `tgju-api-go` but the package is called `tgju`, so the alias above
is worth writing even though Go does not need it — it makes the call sites read the way
you would say them out loud.

## The client

Build one and keep it. A `Client` owns the HTTP connection pool and the snapshot cache,
and both are wasted when a client is created per request.

```go
client := tgju.New(
    tgju.WithTimeout(10*time.Second),
    tgju.WithCacheTTL(time.Minute),
    tgju.WithUserAgent("acme-pricing/2.1"),
)
```

It is safe for concurrent use.

## Fetching

```go
snap, err := client.Gold(ctx)      // or Currency, Coin
snap, err := client.Fetch(ctx, tgju.Coin)
all,  err := client.FetchAll(ctx)  // every market, concurrently
```

`FetchAll` is all or nothing: the first failure is returned and the partial result is
discarded. A caller that wants "whatever succeeded" should loop over `Fetch` and decide
for itself what a hole in the data means.

## Reading a snapshot

A `Snapshot` is one board at one moment, grouped into the tables the site renders.

```go
snap.Len()                  // how many instruments
snap.Items()                // flat slice
snap.Keys()                 // just the keys
snap.Lookup("geram18")      // (Item, bool)
snap.Category("قیمت نقره")   // (Category, bool)
snap.IsEmpty()

for item := range snap.All() {   // an iterator; no slice is allocated
    fmt.Println(item.Key, item.Price.Text)
}
```

## Reading an item

```go
type Item struct {
    Key        string   // "price_dollar_rl" — stable, store this
    Title      string   // "دلار"
    Market     Market   // "currency"
    Category   string   // the caption of the table it sat in
    Price      Amount
    Low, High  Amount   // the day's extremes
    Change     Change
    Time       string   // "11:49:45", or "24 مرداد" for a stale board
    ProfileURL string
}
```

`Amount` keeps both halves of a price, because they answer different questions:

```go
item.Price.Text     // "1,864,000" — what you show a Persian speaking user
item.Price.Value    // 1864000     — what you compare and store
item.Price.Rial()   // int64
item.Price.Toman()  // 186400
```

`Change` is unsigned, because tgju renders it that way. The direction lives in `Status`:

```go
item.Change.Status    // tgju.StatusLow, tgju.StatusHigh, or tgju.StatusUnknown
item.Change.Percent   // 0.32, always positive
item.Change.Amount    // an Amount, always positive
item.Change.Signum()  // -1, +1 or 0 — use this for arithmetic
```

## Looking up one instrument

When a configuration file names instruments but not the boards they live on:

```go
item, err := client.Item(ctx, "geram18")                 // searches every board
item, err := client.Item(ctx, "geram18", tgju.Gold)      // or just one
```

With the cache on this costs at most one fetch per market per TTL, so it is a reasonable
call to make per request in a service.

## Caching

Snapshots are cached for `WithCacheTTL` — thirty seconds by default — and **concurrent
misses for the same market are collapsed into a single request upstream**. That second
property is the one that matters under load: without it, a cold cache with a hundred
callers sends a hundred requests to tgju.

```go
client.CacheTTL()                    // what it is set to
client.Invalidate(tgju.Gold)         // drop one board
client.Invalidate()                  // drop everything
tgju.New(tgju.WithCacheTTL(0))       // turn it off, and the collapsing with it
```

Leave it on when the client backs an HTTP API.

## Errors

Every failure is a `*tgju.Error` wrapping a sentinel, so both the category and the detail
survive:

```go
switch {
case errors.Is(err, tgju.ErrParse):
    // tgju changed its markup. Retrying will not help; alert someone.

case errors.Is(err, tgju.ErrNotFound):
    // no instrument with that key

default:
    var tgjuErr *tgju.Error
    if errors.As(err, &tgjuErr) && tgjuErr.Temporary() {
        // a bad moment upstream; worth trying again
    }
}
```

The full list is on the [Errors](Errors) page.

## Options

| Option | Default | Notes |
| --- | --- | --- |
| `WithHTTPClient` | `&http.Client{}` | Plug in tracing, a proxy, or a stub |
| `WithBaseURL` | `https://www.tgju.org` | A mirror, or a test server |
| `WithTimeout` | `20s` | Bounds one fetch, retries included |
| `WithCacheTTL` | `30s` | `0` disables the cache and the request collapsing |
| `WithRetry` | 3 attempts, 300ms doubling | `RetryPolicy{}` disables retrying |
| `WithUserAgent` | a browser string naming this library | tgju rejects clients that do not look like browsers |
| `WithHeader` | — | Call repeatedly to set several |
| `WithMaxBodyBytes` | 16 MiB | Protects a long lived service from a broken upstream |
| `WithLogger` | discards | A `*slog.Logger`; debug for requests, warn for failures |
| `WithClock` | `time.Now` | For tests |

## Testing against it

Point the client at your own server. The library treats the base URL as the only thing it
knows about tgju:

```go
srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    http.ServeFile(w, r, "testdata/gold.html")
}))
defer srv.Close()

client := tgju.New(tgju.WithBaseURL(srv.URL), tgju.WithCacheTTL(0))
```

Turn the cache off in tests unless the cache is what you are testing.
