# How the scraper works

tgju.org has no API. The prices are read out of the HTML its pages are built from, which
means this project has exactly one long term risk: the day tgju restyles a page. Everything
below is about making that day cheap.

## The shape of the pages

All three boards render the same markup. One `<table class="... market-table ...">` per
section, and one row per instrument:

```html
<table class="data-table market-table">
  <thead><tr>
    <th>عنوان</th><th>قیمت زنده</th><th>تغییر</th>
    <th>کمترین</th><th>بیشترین</th><th>زمان</th><th></th>
  </tr></thead>
  <tbody>
    <tr data-market-nameslug="price_dollar_rl" data-price="1,864,000">
      <th><span class="mini-flag flag-us"></span>دلار</th>
      <td class="nf">1,864,000</td>
      <td class="nf"><span class="low">(0.32%) 6,050</span></td>
      <td>1,860,800</td>
      <td>1,869,100</td>
      <td>۱۱:۴۹:۴۵</td>
      <td class="chart-td"><a href="profile/price_dollar_rl"></a></td>
    </tr>
  </tbody>
</table>
```

Three details are worth knowing:

1. **`data-market-nameslug` is the identity.** It is the key you store. The parser falls
   back to the last segment of the chart link, and drops a row that has neither — a
   spacer, an advertisement, a footer.
2. **The direction lives in a class, not in the number.** `(0.32%) 6,050` is unsigned; only
   `class="low"` on the inner `<span>` says the price fell. That is why `Change.Percent`
   and `Change.Amount` are always positive and `Change.Status` carries the sign.
3. **The time column is not always a time.** Actively traded instruments show a clock;
   stale ones show a Persian date such as `۲۴ مرداد`. It is passed through as text, because
   the site gives no year and no timezone, and inventing either would be a lie.

## Where the fragility is contained

```
internal/scrape   ← the only package that knows what HTML looks like
internal/dom      ← a query layer, no tgju knowledge
internal/numfmt   ← Persian digits, separators, number parsing
convert.go        ← maps a scraped row onto the domain model
```

`internal/scrape` deals in tables, cells and slugs and knows nothing about currencies,
gold or rials. It returns text. `convert.go` turns that text into an `Item`. When tgju
changes something, the fix is almost always in `internal/scrape` alone, and the fixtures
prove it.

## What the parser will and will not tolerate

**Tolerated:**

- Columns reordered. The layout is read from the `<thead>` captions — `قیمت زنده`,
  `تغییر`, `کمترین`, `بیشترین`, `زمان` — so swapping the low and the high upstream
  swaps them in the parser too, rather than silently mislabelling every price.
- Extra classes on the table, extra attributes on a row, new columns at the end.
- A missing `<thead>`. The historical column order is used as a fallback.
- Persian, Arabic-Indic and ASCII digits, `٬` and `,` separators, `٫` and `.` decimals,
  zero-width joiners in titles.
- A row with no chart link, or no daily extremes.

**Not tolerated, by design:**

- The `market-table` class disappearing. That produces `ErrParse` immediately rather than
  a half-parsed page, because a scraper that guesses is worse than one that stops.
- A page with tables but no rows. That is `ErrEmpty`, which is a different problem from
  `ErrParse` and is reported as such.

## When it does break

The [`tgju changed its markup`](https://github.com/amiranmanesh/tgju-api-go/issues/new?template=upstream_change.yml)
issue template asks for the one thing that makes a fix a single commit: the new markup.

The repair loop:

```bash
make fixtures          # refetch and re-trim the saved pages
go test ./...          # the failures point at what moved
# fix internal/scrape, and the pinned values in the tests if prices changed
make ci
```

`make fixtures` writes to `internal/fixture/testdata/`. The diff **is** the review: it
should be small and it should be about markup. If it is large, look at what else changed
on the page before trusting it.

## Why the CI has a job that is allowed to fail

Every unit test runs against saved pages, which means the whole suite can be green while
the live site has moved on. The `live` job in the CI workflow fetches all three boards
from the real tgju.org on every push to `main`. It does not block a merge — a red build
because a third party site is down helps nobody — but it is the canary, and it is the
first thing to look at when someone reports that prices stopped updating.

## Why crypto is missing

`https://www.tgju.org/crypto` builds its table with client-side JavaScript. Reading it
would mean shipping a headless browser, and a headless browser has no place in a library
you import to look up an exchange rate. If tgju ever server-renders that board, adding it
is the ten lines described in [Adding a market](Adding-a-Market).
