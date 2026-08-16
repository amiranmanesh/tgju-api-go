<div align="center">

# tgju-api-go

**Live Iranian currency, gold and coin prices — as a Go library *and* as a JSON API.**

[![CI](https://github.com/amiranmanesh/tgju-api-go/actions/workflows/ci.yml/badge.svg)](https://github.com/amiranmanesh/tgju-api-go/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/amiranmanesh/tgju-api-go.svg)](https://pkg.go.dev/github.com/amiranmanesh/tgju-api-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/amiranmanesh/tgju-api-go)](https://goreportcard.com/report/github.com/amiranmanesh/tgju-api-go)
[![Docs](https://img.shields.io/badge/docs-github%20pages-9a6b12)](https://amiranmanesh.github.io/tgju-api-go/)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

[Documentation](https://amiranmanesh.github.io/tgju-api-go/) ·
[API reference](https://amiranmanesh.github.io/tgju-api-go/api.html) ·
[Wiki](https://github.com/amiranmanesh/tgju-api-go/wiki) ·
[Go package](https://pkg.go.dev/github.com/amiranmanesh/tgju-api-go)

</div>

---

[tgju.org](https://www.tgju.org) publishes the exchange rates, gold weights and coin
prices most of Iran quotes, and has no public API. This project reads the price tables its
pages are built from and gives them back two ways:

- **`import`** it, and call `client.Gold(ctx)` in your own process — no HTTP hop, no
  sidecar, no second thing to operate.
- **`docker run`** it, and call `GET /v1/markets/gold` over HTTP — from Python, from
  Node, from a browser, from anything.

Same client behind both, so they cannot disagree about what a price is.

It reimplements [BlackIQ/tgju-api](https://github.com/BlackIQ/tgju-api) in Go, and keeps
that project's response shape under `/api/price/*` so an existing consumer changes one
hostname and nothing else.

## Contents

- [Install](#install)
- [As a library](#as-a-library)
- [As a service](#as-a-service)
- [Both at once](#both-at-once)
- [The CLI](#the-cli)
- [What you get back](#what-you-get-back)
- [Markets](#markets)
- [Errors](#errors)
- [Caching](#caching)
- [Configuration](#configuration)
- [Design](#design)
- [Testing](#testing)
- [Development](#development)
- [Compatibility with the Python API](#compatibility-with-the-python-api)
- [Project layout](#project-layout)
- [Credits](#credits)
- [Licence and fair use](#licence-and-fair-use)

## Install

```bash
# as a module
go get github.com/amiranmanesh/tgju-api-go

# as a binary
go install github.com/amiranmanesh/tgju-api-go/cmd/tgju@latest

# as a container
docker pull ghcr.io/amiranmanesh/tgju-api-go:latest
```

Go 1.26 or newer. One dependency: `golang.org/x/net/html`.

## As a library

```go
package main

import (
    "context"
    "fmt"
    "log"

    tgju "github.com/amiranmanesh/tgju-api-go"
)

func main() {
    client := tgju.New()

    snap, err := client.Gold(context.Background())
    if err != nil {
        log.Fatal(err)
    }

    item, ok := snap.Lookup("geram18")
    if !ok {
        log.Fatal("tgju no longer publishes 18 carat gold")
    }

    fmt.Println(item.Title)             // طلای 18 عیار / 750
    fmt.Println(item.Price.Text)        // 190,542,000
    fmt.Println(item.Price.Toman())     // 1.9054200e+07
    fmt.Println(item.Change.Status)     // low
}
```

Build the client once and keep it: it owns the connection pool and the snapshot cache, and
it is safe for concurrent use.

Ranging over a board, without allocating a slice:

```go
for item := range snap.All() {
    fmt.Printf("%-22s %14s %s\n", item.Key, item.Price.Text, item.Change.Status)
}
```

Looking something up without caring which board it is on:

```go
item, err := client.Item(ctx, "price_dollar_rl")   // searches every market
item, err := client.Item(ctx, "geram18", tgju.Gold) // or just one
```

Everything at once, fetched concurrently:

```go
all, err := client.FetchAll(ctx)   // map[tgju.Market]tgju.Snapshot
```

More in the [library guide](https://github.com/amiranmanesh/tgju-api-go/wiki/Library-Guide).

## As a service

```bash
docker run -p 8080:8080 ghcr.io/amiranmanesh/tgju-api-go:latest
```

```bash
curl localhost:8080/v1/markets                        # what boards exist
curl localhost:8080/v1/markets/gold                   # a whole board
curl localhost:8080/v1/items/price_dollar_rl          # one instrument
curl localhost:8080/v1/snapshot                       # everything

# filtered
curl 'localhost:8080/v1/markets/currency/items?keys=price_dollar_rl,price_eur'

# the original Python API's shape, unchanged
curl localhost:8080/api/price/currency
```

| Method | Path | Returns |
| --- | --- | --- |
| GET | `/v1/markets` | The supported boards |
| GET | `/v1/markets/{market}` | One board, grouped into categories |
| GET | `/v1/markets/{market}/items` | One board, flattened |
| GET | `/v1/markets/{market}/items/{key}` | One instrument of one board |
| GET | `/v1/items/{key}` | One instrument, across every board |
| GET | `/v1/snapshot` | Every board in one response |
| GET | `/api/price/{currency,gold,coin}` | The original API's shape |
| GET | `/healthz` `/readyz` | Liveness, readiness |
| GET | `/metrics` | Prometheus |
| GET | `/openapi.yaml` `/docs` | The description, and a reference page |

`{market}` accepts `currency`, `gold` and `coin`, plus the aliases `fx`, `gold-chart`,
`coins`, `ارز`, `طلا`, `سکه`, case insensitively.

Full reference: **[amiranmanesh.github.io/tgju-api-go/api.html](https://amiranmanesh.github.io/tgju-api-go/api.html)**.

## Both at once

The API is an ordinary `http.Handler`, which is the point of the whole design: it can be
one subtree of a service that also uses the library directly, with a shared cache between
them.

```go
client := tgju.New(tgju.WithCacheTTL(30 * time.Second))

mux := http.NewServeMux()

// the library half: your own logic, in process
mux.HandleFunc("GET /shop/quote", func(w http.ResponseWriter, r *http.Request) {
    gold, err := client.Item(r.Context(), "geram18", tgju.Gold)
    if err != nil {
        http.Error(w, "prices unavailable", http.StatusServiceUnavailable)
        return
    }
    fmt.Fprintf(w, "%.0f toman per gram\n", gold.Price.Toman())
})

// the service half: the ready made API, mounted under a prefix
mux.Handle("/prices/", http.StripPrefix("/prices", server.New(client)))
```

A quote and an API call arriving in the same window cost **one** fetch from tgju between
them. Runnable version: [`examples/embed`](examples/embed).

## The CLI

```
tgju serve [flags]           start the HTTP API
tgju get <market> [flags]    print one board
tgju item <key> [flags]      print one instrument
tgju watch <key> [flags]     follow one instrument until interrupted
tgju markets                 list the supported boards
tgju version                 print the version
```

```console
$ tgju get gold --unit toman
قیمت طلا
KEY             TITLE               PRICE       LOW         HIGH        CHANGE   TIME
geram18         طلای 18 عیار / 750  19,054,200  19,039,400  19,069,200  ▼ 0.13%  11:49:55
gold_740k       طلای 18 عیار / 740  18,800,100  18,785,600  18,814,900  ▼ 0.13%  11:49:55
geram24         طلای 24 عیار        25,405,300  25,385,600  25,425,300  ▼ 0.13%  11:49:55

قیمت نقره
KEY         TITLE         PRICE    LOW      HIGH     CHANGE   TIME
silver_925  گرم نقره 925  376,590  374,650  380,470  ▲ 0.47%  11:44:25
```

```bash
tgju get currency --format json | jq '.categories[].items[] | select(.change.percent > 1)'
tgju get coin --format csv > coins.csv
tgju watch price_dollar_rl --interval 30s
```

Exit codes: `0` success, `1` failure, `2` bad usage, `3` tgju could not be read — so a
script can tell "the site is down" from "you typed it wrong".

## What you get back

```go
type Item struct {
    Key        string   // "price_dollar_rl" — stable across redesigns; store this
    Title      string   // "دلار"
    Market     Market   // "currency"
    Category   string   // the caption of the table it sat in
    Price      Amount
    Low, High  Amount   // the day's extremes
    Change     Change
    Time       string   // "11:49:45", or "24 مرداد" on a stale board
    ProfileURL string
}
```

An `Amount` keeps both halves of a price, because they answer different questions:

```go
item.Price.Text     // "1,864,000" — what you show a Persian speaking user
item.Price.Value    // 1864000     — what you compare, sort and store
item.Price.Toman()  // 186400      — the unit people actually speak in
```

tgju renders the daily change **unsigned**, so the direction lives in a field of its own
rather than in the sign of a number:

```go
item.Change.Status    // StatusLow, StatusHigh, or StatusUnknown
item.Change.Percent   // 0.32, always positive
item.Change.Signum()  // -1, +1 or 0 — for arithmetic
```

Over the wire:

```json
{
  "key": "price_dollar_rl",
  "title": "دلار",
  "market": "currency",
  "category": "عنوان",
  "price": { "text": "1,864,000", "value": 1864000 },
  "low":   { "text": "1,860,800", "value": 1860800 },
  "high":  { "text": "1,869,100", "value": 1869100 },
  "change": {
    "status": "low",
    "percent": 0.32,
    "amount": { "text": "6,050", "value": 6050 }
  },
  "time": "11:49:45",
  "profile_url": "https://www.tgju.org/profile/price_dollar_rl"
}
```

**Prices are in rial**, as tgju quotes them.

## Markets

| Market | Board | Source |
| --- | --- | --- |
| `currency` | ارز | [tgju.org/currency](https://www.tgju.org/currency) |
| `gold` | طلا و نقره | [tgju.org/gold-chart](https://www.tgju.org/gold-chart) |
| `coin` | سکه | [tgju.org/coin](https://www.tgju.org/coin) |

The crypto board is built by client-side JavaScript and is deliberately absent: reading it
would need a headless browser, and a headless browser has no place in a library you import
to look up an exchange rate.

Key catalogue: [Instrument keys](https://github.com/amiranmanesh/tgju-api-go/wiki/Instrument-Keys).

## Errors

Every failure is a `*tgju.Error` wrapping a sentinel, so both the category and the detail
survive the trip:

```go
switch {
case errors.Is(err, tgju.ErrParse):
    // tgju changed its markup. Retrying will never work; this needs a release.
    alert(err)

case errors.Is(err, tgju.ErrNotFound):
    return errNoSuchInstrument

default:
    var tgjuErr *tgju.Error
    if errors.As(err, &tgjuErr) && tgjuErr.Temporary() {
        return retryLater(err)
    }
}
```

The distinction that matters is between **"tgju is down"** and **"tgju changed"**. The
first is worth a retry; the second is a busy loop. They are separate sentinels, separate
API codes (`upstream_unavailable` and `upstream_changed`), and worth separate alerts.

Full table: [Errors](https://github.com/amiranmanesh/tgju-api-go/wiki/Errors).

## Caching

Snapshots are held for thirty seconds by default, and **concurrent misses for the same
board are collapsed into a single request upstream**. That second property is the one that
matters under load: without it, a cold cache with a hundred callers sends a hundred
requests to tgju.

```go
tgju.New(tgju.WithCacheTTL(time.Minute))
tgju.New(tgju.WithCacheTTL(0))        // off, and the collapsing with it
client.Invalidate(tgju.Gold)          // drop one board
```

Responses carry `Cache-Control: public, max-age=<remaining TTL>`, so a CDN in front of the
service repeats the same policy one layer out.

## Configuration

Every flag of `tgju serve` has an environment variable — upper case, `TGJU_` prefixed,
dashes as underscores. The flag wins when both are set.

```bash
TGJU_ADDR=:8080
TGJU_CACHE_TTL=30s
TGJU_TIMEOUT=20s
TGJU_RETRIES=3
TGJU_RATE_LIMIT=20
TGJU_RATE_BURST=40
TGJU_CORS=*
TGJU_LOG_LEVEL=info
TGJU_LOG_JSON=true
```

The complete list is in [`docker-compose.yml`](docker-compose.yml) and in
`tgju serve -h`.

## Design

**The fragile part is small, and it is isolated.** `internal/scrape` knows about tables,
header cells and slugs, and nothing about currencies, gold or rials. It returns text.
`convert.go` turns that text into domain types. A tgju redesign is a change to one
package, proved by fixtures.

**The parser follows the header, not the column index.** The layout is read from the
Persian captions (`قیمت زنده`, `کمترین`, `بیشترین`, …), so a reshuffle upstream cannot
silently swap the daily low with the daily high.

**Persian numerals are a first-class concern.** Persian and Arabic-Indic digits, `٬` and
`,` grouping, `٫` and `.` decimals, and zero-width joiners are all normalised before
anything is parsed — and the parser is fuzzed.

**Failure modes are distinguished.** "Cannot reach tgju", "tgju answered with a status",
"the page will not parse", "the page parsed but was empty" and "no such instrument" are
five different errors, because they call for five different reactions.

**One dependency.** `golang.org/x/net/html`. No web framework — `net/http`'s router does
methods and path variables. No metrics client — the Prometheus text format is a few lines
of `fmt.Fprintf`. No YAML parser — the OpenAPI document is checked structurally by a tool
in `internal/cmd/checkspec`. `make deps-check` keeps it that way.

**The HTTP layer is a handler, not a program.** Nothing in `server/` opens a socket or
reads the environment. That is what lets it be mounted inside somebody else's service.

## Testing

```bash
make test     # race detector
make cover    # coverage
make fuzz     # fuzz the number parsers
make live     # hit the real tgju.org
```

Tests run against **saved tgju pages** in `internal/fixture/testdata`, shared by the
parser, client, server and CLI suites, so all four agree on what tgju looks like. Refresh
them with `make fixtures`; the diff is the review.

Because the fixtures are frozen, the suite can be green while the live site has moved on.
That gap is covered by a CI job that fetches all three boards from the real tgju.org on
every push to `main`. It is allowed to fail — a red build because a third party is down
helps nobody — and it is the canary that goes off first when tgju redesigns a page.

## Development

```bash
git clone https://github.com/amiranmanesh/tgju-api-go
cd tgju-api-go
make ci       # lint, dependency policy, spec check, tests
make run      # serve on :8080
make help     # every target
```

See [CONTRIBUTING.md](CONTRIBUTING.md).

## Compatibility with the Python API

`/api/price/currency` and `/api/price/gold` return exactly what
[BlackIQ/tgju-api](https://github.com/BlackIQ/tgju-api) returned.
`/api/price/coin` extends the same shape to coins.

One difference: absent values are `""` rather than `null`. If your client checks
`is None`, change it to a falsiness check — that is the whole migration.

The [migration guide](https://github.com/amiranmanesh/tgju-api-go/wiki/Migrating-from-the-Python-API)
has the details, including what the request-log database was for and why there isn't one.

## Project layout

```
.                      the library: Client, Snapshot, Item, options, errors
├── server/            the HTTP API as an http.Handler, plus openapi.yaml
├── cmd/tgju/          the binary: serve, get, item, watch, markets, version
├── internal/
│   ├── scrape/        the HTML parser — everything fragile lives here
│   ├── dom/           a query layer over golang.org/x/net/html
│   ├── numfmt/        Persian digits and number parsing
│   ├── fixture/       saved tgju pages, shared by every test suite
│   └── cmd/           maintenance tools: fixtures, checkspec, healthcheck
├── examples/          basic, embed (both halves at once), alert
├── docs/              the GitHub Pages site
└── wiki/              the wiki, published by a workflow
```

## Credits

The endpoint shape, the field names and the choice of boards come from
[BlackIQ/tgju-api](https://github.com/BlackIQ/tgju-api) by
[@BlackIQ](https://github.com/BlackIQ). The `status`, `low_price` and `high_price` fields
were [@fatehi-develop](https://github.com/fatehi-develop)'s idea. This is a
reimplementation rather than a fork, and the compatibility layer exists so that work is
not wasted.

The prices themselves belong to [tgju.org](https://www.tgju.org).

## Licence and fair use

[MIT](LICENSE).

This project is **not affiliated with, endorsed by, or connected to tgju.org**. It reads
their public pages and makes no claim about the accuracy of the data it relays — if tgju
is wrong, this is wrong.

If you run it in front of real users: keep the cache on, do not remove the rate limiter,
and read tgju's terms of service first. A scraper that is polite costs its source almost
nothing; one that is not gets everybody blocked.
