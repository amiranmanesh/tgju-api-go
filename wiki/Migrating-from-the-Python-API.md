# Migrating from BlackIQ/tgju-api

This project reimplements [BlackIQ/tgju-api](https://github.com/BlackIQ/tgju-api), a
FastAPI service that scrapes the same site. If you are running that today, the move is a
hostname change.

## What is byte-compatible

| Original | Here |
| --- | --- |
| `GET /api/price/currency` | Same path, same body, same field names |
| `GET /api/price/gold` | Same path, same body, same field names |

```json
[
  {
    "title": "دلار",
    "price": "1,864,000",
    "key": "price_dollar_rl",
    "status": "low",
    "low_price": "1,860,800",
    "high_price": "1,869,100"
  }
]
```

Currency is a flat array; gold is an array of `{title, prices[]}` categories. Exactly as
before.

## What changed

**Absent values are `""`, not `null`.** The Python service emitted `null` for a missing
status or extreme; this one emits an empty string. If your client does
`if item["status"] is None`, change it to a falsiness check. That is the only breaking
difference, and it is the one to grep for before you switch.

**There is no database.** The original logged every request to Postgres through a
middleware. This service logs to stdout as structured JSON and exposes Prometheus metrics,
which is what that table was being used for. Nothing to provision, nothing to migrate,
nothing to back up.

**`/docs` is not Swagger UI.** It is a hand-written page that needs no CDN, so it works
inside a container on a closed network. The OpenAPI document is at `/openapi.yaml`; point
Swagger UI, Redoc or Scalar at it if you want the interactive version.

## What you gain

**`/api/price/coin`** — the same shape, extended to the coin board.

**`/v1/…`** — the same data with the parts the string-only shape had to drop:

```json
{
  "key": "price_dollar_rl",
  "title": "دلار",
  "market": "currency",
  "category": "عنوان",
  "price": { "text": "1,864,000", "value": 1864000 },
  "low":   { "text": "1,860,800", "value": 1860800 },
  "high":  { "text": "1,869,100", "value": 1869100 },
  "change": { "status": "low", "percent": 0.32,
              "amount": { "text": "6,050", "value": 6050 } },
  "time": "11:49:45",
  "profile_url": "https://www.tgju.org/profile/price_dollar_rl"
}
```

The numbers arrive parsed, so nobody downstream writes another comma-stripping regex.
`change` and `time` were not exposed at all before.

**Lookups.** `GET /v1/items/price_dollar_rl` returns one instrument without fetching and
filtering a whole board client-side.

**Caching and request collapsing.** The original fetched tgju on every request. This one
holds a snapshot for thirty seconds and collapses concurrent misses into one request, so
the load tgju sees stops scaling with your traffic.

**A library.** If your consumer is itself in Go, you can delete the HTTP hop entirely —
see the [Library guide](Library-Guide).

## The steps

1. Run the new service beside the old one.
2. Diff the two responses for the endpoints you use.
   ```bash
   diff <(curl -s old/api/price/currency | jq -S .) \
        <(curl -s new/api/price/currency | jq -S .)
   ```
   Expect differences only in the prices themselves and in `null` versus `""`.
3. Grep your clients for `is None` / `=== null` on `status`, `low_price` and `high_price`.
4. Switch the hostname.
5. Move to `/v1` when you next touch the client.

## Things the original had that this does not

- **The request log table.** Deliberate; see above.
- **Alembic migrations.** There is no database to migrate.
- **Notebooks.** Out of scope.

## Credit

The endpoint shape, the field names and the choice of boards come from the original
project by [@BlackIQ](https://github.com/BlackIQ), and the `status` / `low_price` /
`high_price` fields were [@fatehi-develop](https://github.com/fatehi-develop)'s idea. This
is a reimplementation, not a fork; the compatibility layer exists so that work is not
wasted.
