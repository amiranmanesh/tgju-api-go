# tgju-api-go

Live currency, gold and coin prices from [tgju.org](https://www.tgju.org) — importable as a
Go module, runnable as a JSON API.

> These pages are generated from the [`wiki/`](https://github.com/amiranmanesh/tgju-api-go/tree/main/wiki)
> directory of the repository and are published by a workflow. Edit them there, in a pull
> request; edits made in the wiki UI are overwritten on the next push to `main`.

## Start here

| If you want to… | Read |
| --- | --- |
| Call it from Go code | [Library guide](Library-Guide) |
| Run the JSON API | [Running the service](Running-the-Service) |
| Deploy it | [Deployment](Deployment) |
| Know what `geram18` means | [Instrument keys](Instrument-Keys) |
| Fix it after tgju redesigns a page | [How the scraper works](How-the-Scraper-Works) |
| Add a fourth market | [Adding a market](Adding-a-Market) |
| Move off the Python service | [Migrating from BlackIQ/tgju-api](Migrating-from-the-Python-API) |
| Understand a `502` | [Errors](Errors) |

## The thirty second version

```go
client := tgju.New()

snap, err := client.Gold(ctx)
if err != nil {
    return err
}

item, ok := snap.Lookup("geram18")
fmt.Println(item.Title, item.Price.Text, item.Price.Toman())
```

```bash
docker run -p 8080:8080 ghcr.io/amiranmanesh/tgju-api-go:latest
curl localhost:8080/v1/markets/gold
```

## What this is, and what it is not

tgju.org has no public API. This project reads the price tables its pages are built from,
which is an honest description of both what it does and what its risks are:

- **It will break when tgju restyles a page.** That is not a defect in the design, it is
  the nature of the technique. The project is built so that the break is loud, narrow and
  quick to fix — see [How the scraper works](How-the-Scraper-Works).
- **It relays numbers, it does not verify them.** If tgju is wrong, this is wrong.
- **It is not affiliated with tgju.org.** Read their terms before putting this in front of
  paying users, and keep the cache on so you are not making a request per customer.

## Layout of the repository

| Path | What lives there |
| --- | --- |
| `/` | The public library: `Client`, `Snapshot`, `Item`, options, errors |
| `server/` | The HTTP API as an `http.Handler`, plus the OpenAPI document |
| `cmd/tgju/` | The binary: `serve`, `get`, `item`, `watch`, `markets`, `version` |
| `internal/scrape/` | The HTML parser. Everything fragile is here |
| `internal/dom/` | A small query layer over `golang.org/x/net/html` |
| `internal/numfmt/` | Persian digits and number parsing |
| `internal/fixture/` | Saved tgju pages, shared by every test suite |
| `examples/` | Runnable programs, including one that does both halves at once |
| `docs/` | The GitHub Pages site |
| `wiki/` | These pages |
