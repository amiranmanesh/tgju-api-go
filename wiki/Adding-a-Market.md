# Adding a market

Every board tgju server-renders uses the same table markup, so adding one is a
registration plus a fixture. This is the whole procedure.

## 1. Check the page is server-rendered

```bash
curl -sL -A "Mozilla/5.0" https://www.tgju.org/<page> | grep -c 'market-table'
```

Zero means the table is built by JavaScript and this project cannot read it. That is why
there is no crypto market.

## 2. Register it

In `market.go`:

```go
const (
    Currency Market = "currency"
    Gold     Market = "gold"
    Coin     Market = "coin"
    Stock    Market = "stock"     // new
)

var sources = map[Market]source{
    Currency: {path: "/currency", label: "ارز"},
    Gold:     {path: "/gold-chart", label: "طلا و نقره"},
    Coin:     {path: "/coin", label: "سکه"},
    Stock:    {path: "/stock", label: "بورس"},   // new
}
```

Then add the aliases people will actually type, in `ParseMarket`:

```go
case "stock", "stocks", "بورس":
    return Stock, nil
```

`Markets()` reads the registry, so the CLI, `/v1/markets`, `FetchAll` and `Item` all pick
the new board up with no further changes.

## 3. Add a fixture

In `internal/cmd/fixtures/main.go`:

```go
var files = map[tgju.Market]string{
    // ...
    tgju.Stock: "stock.html",
}
```

and in `internal/fixture/fixture.go`:

```go
var Paths = map[string]string{
    // ...
    "/stock": "testdata/stock.html",
}
```

Then:

```bash
make fixtures
```

## 4. Add the tests

At minimum, extend the table in `TestFetchParsesEveryBoard` (`client_test.go`) and
`TestTablesOnSavedPages` (`internal/scrape/scrape_test.go`) with the new board, a known
key and its title.

`TestMarkets` will fail until you add the market to its expected list — that failure is
the point: the ordering of `Markets()` is part of the API.

## 5. Document it

- `server/openapi.yaml` — the `Market` schema's `enum`.
- `docs/index.html` — the markets table.
- `wiki/Instrument-Keys.md` — a few of the new keys.
- `CHANGELOG.md` — under `Unreleased`.

`make ci` runs `checkspec`, which will tell you if the OpenAPI document has drifted, and
`TestOpenAPICoversEveryRoute` fails if a new route is undescribed.

## A note on the legacy endpoints

`/api/price/*` exists to be byte-compatible with the Python service. Do **not** add a new
market there unless the shape genuinely fits — `/api/price/coin` was added because coins
group into categories exactly like gold does. New boards belong under `/v1`.
