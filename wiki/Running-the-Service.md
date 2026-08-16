# Running the service

Three ways, same handler behind all of them.

## The container

```bash
docker run -p 8080:8080 ghcr.io/amiranmanesh/tgju-api-go:latest
open http://localhost:8080/docs
```

## The binary

```bash
go install github.com/amiranmanesh/tgju-api-go/cmd/tgju@latest
tgju serve --addr :8080
```

## Inside a service you already run

This is the reason the API is a plain `http.Handler` rather than a program:

```go
client := tgju.New(tgju.WithCacheTTL(30 * time.Second))

mux := http.NewServeMux()
mux.Handle("/prices/", http.StripPrefix("/prices", server.New(client)))
mux.HandleFunc("GET /orders", myOwnHandler)
```

The same `client` can be used directly by the rest of your code, so a handler and a
background job share one cache and one connection pool. See `examples/embed`.

## Endpoints

| Method | Path | What it returns |
| --- | --- | --- |
| GET | `/v1/markets` | The supported boards, with their sources |
| GET | `/v1/markets/{market}` | One board, grouped into categories |
| GET | `/v1/markets/{market}/items` | One board, flattened |
| GET | `/v1/markets/{market}/items/{key}` | One instrument of one board |
| GET | `/v1/items/{key}` | One instrument, searched across every board |
| GET | `/v1/snapshot` | Every board in one response |
| GET | `/api/price/currency` | The original Python API's shape |
| GET | `/api/price/gold` | The original Python API's shape |
| GET | `/api/price/coin` | The same shape, extended to coins |
| GET | `/healthz` | Liveness. Never touches tgju |
| GET | `/readyz` | Readiness. Fetches through the cache |
| GET | `/metrics` | Prometheus text exposition |
| GET | `/openapi.yaml` | The API description |
| GET | `/docs` | A self-contained reference page |

`{market}` accepts `currency`, `gold`, `coin`, and the aliases `fx`, `gold-chart`,
`coins`, `ارز`, `طلا`, `سکه`. It is case insensitive.

## Query parameters

```bash
# only two instruments
curl 'localhost:8080/v1/markets/currency/items?keys=price_dollar_rl,price_eur'

# only the silver table (the title is Persian, so URL-encode it)
curl -G localhost:8080/v1/markets/gold --data-urlencode 'category=قیمت نقره'
```

## Configuration

Every flag of `tgju serve` has an environment variable: upper case, `TGJU_` prefixed,
dashes as underscores. The flag wins when both are set.

| Flag | Variable | Default | Notes |
| --- | --- | --- | --- |
| `--addr` | `TGJU_ADDR` | `:8080` | |
| `--cache-ttl` | `TGJU_CACHE_TTL` | `30s` | `0` turns the cache off |
| `--timeout` | `TGJU_TIMEOUT` | `20s` | One fetch, retries included |
| `--retries` | `TGJU_RETRIES` | `3` | Total attempts |
| `--request-timeout` | `TGJU_REQUEST_TIMEOUT` | `30s` | One API request |
| `--rate-limit` | `TGJU_RATE_LIMIT` | `20` | Per client, per second. `0` disables |
| `--rate-burst` | `TGJU_RATE_BURST` | `40` | |
| `--cors` | `TGJU_CORS` | `*` | Comma separated; empty sends no headers |
| `--metrics` | `TGJU_METRICS` | `true` | |
| `--base-url` | `TGJU_BASE_URL` | `https://www.tgju.org` | A mirror, or a test server |
| `--shutdown-grace` | `TGJU_SHUTDOWN_GRACE` | `15s` | Drain time on SIGTERM |
| `--log-level` | `TGJU_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `--log-json` | `TGJU_LOG_JSON` | `false` | The image sets it to `true` |

## Caching, and being a good citizen

The service exists to sit between your users and a site that would rather not be scraped.
Leave the cache on. With a thirty second TTL and request collapsing, a thousand callers a
minute produce two requests to tgju.org per board, not a thousand.

Responses carry `Cache-Control: public, max-age=<remaining TTL>`, so a CDN in front of the
service extends the same policy one layer further out.

## Operations

- **Liveness** `/healthz` — answers as long as the process runs.
- **Readiness** `/readyz` — fetches the currency board and answers `503` when tgju cannot
  be reached or parsed. Deliberately separate: an orchestrator should stop sending traffic
  to an instance that cannot reach upstream, but restarting it fixes nothing.
- **Request IDs** — an `X-Request-Id` you send is echoed when it is safe to; otherwise one
  is minted. It appears in every log line and in every error body.
- **Metrics** — `tgju_http_requests_total` and `tgju_http_request_duration_seconds`,
  labelled by route pattern rather than by path, so the cardinality stays bounded.
- **Shutdown** — `SIGTERM` drains in flight requests for `--shutdown-grace` before exiting.
