# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this
project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

Nothing yet.

## [1.0.0] — 2026-08-16

The first release: a Go reimplementation of
[BlackIQ/tgju-api](https://github.com/BlackIQ/tgju-api), usable both as a library and as a
service.

### Added

**The library** (`github.com/amiranmanesh/tgju-api-go`)

- `Client` with `Fetch`, `Currency`, `Gold`, `Coin`, `FetchAll` and `Item`, safe for
  concurrent use.
- `Snapshot`, `Category`, `Item`, `Amount`, `Change` and `Status` as the domain model.
  `Amount` carries both tgju's own rendering and the parsed number; `Amount.Toman()`
  converts from the rial the site quotes in.
- `Snapshot.All()` as a range-over-func iterator, plus `Items`, `Keys`, `Lookup`,
  `Category`, `Len` and `IsEmpty`.
- An in-memory snapshot cache that also collapses concurrent misses for the same market
  into a single upstream request. `WithCacheTTL(0)` disables both.
- Retry with exponential backoff on transport failures, 5xx, 429 and 408; no retry on
  anything a repeat cannot fix.
- Options: `WithHTTPClient`, `WithBaseURL`, `WithTimeout`, `WithCacheTTL`, `WithRetry`,
  `WithUserAgent`, `WithHeader`, `WithMaxBodyBytes`, `WithLogger`, `WithClock`.
- Typed errors: `*Error` wrapping `ErrUnknownMarket`, `ErrRequest`, `ErrUnexpectedStatus`,
  `ErrParse`, `ErrEmpty`, `ErrNotFound` and `ErrTooLarge`, with `Error.Temporary()`
  separating a bad moment upstream from a permanent break.
- Markets `currency`, `gold` and `coin`, resolvable from a string by `ParseMarket`
  including Persian and colloquial aliases.

**The HTTP API** (`server`)

- An `http.Handler`, mountable under any prefix inside an existing service.
- `/v1/markets`, `/v1/markets/{market}`, `/v1/markets/{market}/items`,
  `/v1/markets/{market}/items/{key}`, `/v1/items/{key}` and `/v1/snapshot`.
- `?keys=` and `?category=` filters.
- `/api/price/currency` and `/api/price/gold` reproducing the original Python service's
  response shape byte for byte, plus `/api/price/coin` extending it.
- `/healthz` and `/readyz` as separate probes: readiness fetches upstream, liveness never
  does.
- `/metrics` in the Prometheus text format, labelled by route pattern rather than path.
- `/openapi.yaml` and a `/docs` page that needs no CDN and works offline.
- Middleware: panic recovery, request IDs, structured access logs, CORS, a per-client
  token-bucket rate limiter, and a per-request deadline.
- One error body shape with stable machine-readable codes, and no upstream URLs in
  messages.

**The binary** (`cmd/tgju`)

- `serve`, `get`, `item`, `watch`, `markets` and `version`.
- `table`, `json` and `csv` output; `rial`, `toman` and `raw` units.
- Every flag also readable from a `TGJU_`-prefixed environment variable.
- Distinct exit codes for usage errors and upstream failures.
- Graceful shutdown on SIGTERM.

**Packaging and documentation**

- Multi-architecture container image on `ghcr.io`, built `FROM scratch`, running as
  non-root, with a compiled healthcheck rather than a shell.
- `docker-compose.yml` with every setting spelled out.
- GitHub Actions for CI (lint, tests on three operating systems, fuzzing, govulncheck,
  an image smoke test and a live check against tgju.org), releases (GoReleaser plus a
  multi-arch image with build provenance), GitHub Pages and wiki publishing.
- A wiki kept in `wiki/` and published by a workflow, covering the library, the service,
  deployment, errors, instrument keys, how the scraper works and how to add a market.

### Security

- The one third-party script on the documentation site is pinned to an exact version,
  carries a Subresource Integrity hash and is confined by a Content-Security-Policy.
  GitHub Pages serves every project on an account from one origin, so a script there is
  not sandboxed to this project. `make docs-check` enforces both rules, and the check runs
  in CI and again before the site is published.

### Notes on the reimplementation

- Absent values are `""` rather than `null` in the compatibility endpoints. This is the
  only breaking difference from the Python service.
- The request-log database of the original is replaced by structured logs and Prometheus
  metrics. There is no database.
- The crypto board is not supported: tgju builds it with client-side JavaScript.

[Unreleased]: https://github.com/amiranmanesh/tgju-api-go/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/amiranmanesh/tgju-api-go/releases/tag/v1.0.0
