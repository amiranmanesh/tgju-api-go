// Package tgju reads the live currency, gold and coin boards published by
// tgju.org and returns them as Go values.
//
// The site has no public API, so the package scrapes the price tables its pages
// are built from. That is a deliberate boundary: everything fragile — the class
// names, the column order, the Persian digits — lives behind one small internal
// package, and everything a caller touches is an ordinary struct.
//
// # As a library
//
// Build one [Client] and keep it; it owns the HTTP connection pool and a short
// lived snapshot cache.
//
//	client := tgju.New()
//
//	snap, err := client.Currency(context.Background())
//	if err != nil {
//	    return err
//	}
//
//	dollar, ok := snap.Lookup("price_dollar_rl")
//	if ok {
//	    fmt.Println(dollar.Title, dollar.Price.Text, dollar.Price.Toman())
//	}
//
// A [Snapshot] is one board at one moment, grouped into the [Category] tables
// the site renders. Range over every row with [Snapshot.All], or flatten with
// [Snapshot.Items].
//
// Looking up a single instrument without caring which board it sits on:
//
//	item, err := client.Item(ctx, "geram18")
//
// # As a service
//
// The sibling package [github.com/amiranmanesh/tgju-api-go/server] wraps a
// client in an [net/http.Handler], so the same code either runs as the
// standalone binary in cmd/tgju or mounts inside an existing service:
//
//	mux.Handle("/tgju/", http.StripPrefix("/tgju", server.New(client)))
//
// # Caching and concurrency
//
// A [Client] is safe for concurrent use. Snapshots are cached for
// [DefaultCacheTTL] and concurrent misses for the same market are collapsed into
// one outgoing request, so a busy API server talks to tgju.org once per window
// rather than once per caller. Turn the cache off with WithCacheTTL(0).
//
// # Errors
//
// Every failure is an [*Error] wrapping one of the sentinels — [ErrRequest],
// [ErrUnexpectedStatus], [ErrParse], [ErrEmpty], [ErrNotFound],
// [ErrUnknownMarket] — so both the category and the detail survive:
//
//	if errors.Is(err, tgju.ErrParse) {
//	    // tgju changed its markup; alert, do not retry
//	}
//
//	var tgjuErr *tgju.Error
//	if errors.As(err, &tgjuErr) && tgjuErr.Temporary() {
//	    // worth trying again later
//	}
//
// # Units
//
// tgju quotes currency, gold and coin prices in Iranian rial. [Amount] keeps
// both the site's own rendering and the parsed number, and offers
// [Amount.Toman] for the unit people actually speak in.
package tgju
