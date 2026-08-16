# Errors

The same failure has two faces: a Go error for a library caller, and a status plus a code
for an HTTP client. They are two views of one table.

| Library sentinel | HTTP status | API code | What actually happened | Retry? |
| --- | --- | --- | --- | --- |
| `ErrUnknownMarket` | 404 | `unknown_market` | A market this build does not serve | No |
| `ErrNotFound` | 404 | `not_found` | No instrument with that key | No |
| `ErrRequest` | 502 | `upstream_unavailable` | DNS, TLS, a dropped connection | Yes |
| `ErrUnexpectedStatus` | 502 | `upstream_unavailable` | tgju answered with something other than 200 | Depends |
| `ErrTooLarge` | 502 | `upstream_unavailable` | The response blew past the body limit | No |
| `ErrParse` | 502 | `upstream_changed` | The page fetched but could not be understood | **No** |
| `ErrEmpty` | 502 | `upstream_changed` | The page parsed but held no rows | **No** |
| — | 504 | `timeout` | The request outlived its deadline | Yes |
| — | 429 | `rate_limited` | Too many requests from one client | Yes, later |
| — | 500 | `internal` | A bug | No |

## The one that matters

`upstream_changed` / `ErrParse` is the distinctive failure of a scraper, and it is
separated from every other 502 on purpose:

- **`upstream_unavailable`** means tgju had a bad moment. Back off and try again.
- **`upstream_changed`** means tgju is fine and *this software* is out of date. Retrying
  will never work. Alert a human; the fix is a release.

Alert on them differently. A retry loop on `upstream_changed` is a busy loop.

## In Go

```go
snap, err := client.Gold(ctx)

switch {
case err == nil:

case errors.Is(err, tgju.ErrParse), errors.Is(err, tgju.ErrEmpty):
    // page the on-call engineer, do not retry
    return fmt.Errorf("the tgju scraper needs an update: %w", err)

case errors.Is(err, tgju.ErrNotFound):
    return errStatusNotFound

default:
    var tgjuErr *tgju.Error
    if errors.As(err, &tgjuErr) && tgjuErr.Temporary() {
        return backoffAndRetry(err)
    }
    return err
}
```

`*tgju.Error` carries the context you would otherwise have to reconstruct:

```go
tgjuErr.Op          // "fetch", "parse" or "lookup"
tgjuErr.Market      // which board
tgjuErr.URL         // what was requested
tgjuErr.StatusCode  // what tgju answered, or 0
tgjuErr.Attempts    // how many tries it took before giving up
tgjuErr.Temporary() // whether trying again could plausibly work
```

## Over HTTP

Every failure has one body shape:

```json
{
  "code": "upstream_changed",
  "message": "tgju.org served a page this service could not parse",
  "request_id": "9f2c1a4e7b3d5f8a0c2e4b61"
}
```

Branch on `code`. `message` is prose and may be reworded; `code` will not change without a
major version. `request_id` is what to quote in a bug report — it is in the server logs.

Upstream URLs never appear in a message. They are in the logs, where they belong.

## Exit codes from the CLI

| Code | Meaning |
| --- | --- |
| 0 | Success, or an interrupted `watch` / `serve` |
| 1 | A failure that is not one of the below |
| 2 | You typed something wrong |
| 3 | tgju.org could not be read |

```bash
if ! tgju get gold > prices.json; then
    case $? in
        3) echo "tgju is unreachable; keeping yesterday's file" ;;
        *) exit 1 ;;
    esac
fi
```
