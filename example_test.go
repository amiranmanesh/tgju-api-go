package tgju_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	tgju "github.com/amiranmanesh/tgju-api-go"
	"github.com/amiranmanesh/tgju-api-go/server"
)

// The examples below print nothing, because their output depends on today's
// prices. They are here to be compiled, read on pkg.go.dev, and copied.

func Example() {
	client := tgju.New()

	snap, err := client.Currency(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	dollar, ok := snap.Lookup("price_dollar_rl")
	if !ok {
		log.Fatal("tgju no longer publishes the dollar rate")
	}

	fmt.Println(dollar.Title, dollar.Price.Text, "rial")
	fmt.Println(dollar.Title, dollar.Price.Toman(), "toman")
}

// A client owns a connection pool and a cache, so build it once and keep it for
// the lifetime of the program.
func ExampleNew() {
	client := tgju.New(
		tgju.WithTimeout(10*time.Second),
		tgju.WithCacheTTL(time.Minute),
		tgju.WithUserAgent("acme-pricing/2.1"),
	)
	_ = client
}

// Ranging over a snapshot visits every instrument of every category in the order
// the site published them.
func ExampleSnapshot_All() {
	snap, err := tgju.New().Gold(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	for item := range snap.All() {
		fmt.Printf("%-24s %14s %s\n", item.Key, item.Price.Text, item.Change.Status)
	}
}

// Item searches every board, which is what you want when a configuration file
// names instruments but not the pages they live on.
func ExampleClient_Item() {
	client := tgju.New()

	for _, key := range []string{"price_dollar_rl", "geram18", "sekee"} {
		item, err := client.Item(context.Background(), key)
		if err != nil {
			log.Printf("%s: %v", key, err)
			continue
		}
		fmt.Printf("%s (%s): %s\n", item.Title, item.Market, item.Price.Text)
	}
}

// Every failure carries both a category and its detail, so a caller can decide
// between retrying, alerting and giving up.
func ExampleError() {
	_, err := tgju.New().Gold(context.Background())
	if err == nil {
		return
	}

	switch {
	case errors.Is(err, tgju.ErrParse):
		// tgju changed its markup: retrying will not help.
		log.Fatal("the scraper needs an update: ", err)

	case errors.Is(err, tgju.ErrNotFound):
		log.Print("no such instrument")

	default:
		var tgjuErr *tgju.Error
		if errors.As(err, &tgjuErr) && tgjuErr.Temporary() {
			log.Printf("upstream had a bad moment (status %d), will retry", tgjuErr.StatusCode)
			return
		}
		log.Print(err)
	}
}

// The HTTP API is an ordinary handler, so it can be the whole service or one
// subtree of a larger one.
func ExampleNew_asAService() {
	client := tgju.New(tgju.WithCacheTTL(30 * time.Second))

	mux := http.NewServeMux()
	mux.Handle("/prices/", http.StripPrefix("/prices", server.New(client)))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "my own service")
	})

	log.Fatal(http.ListenAndServe(":8080", mux))
}

// Converting to toman and rounding to the nearest thousand is the kind of thing
// the parsed value is there for.
func ExampleAmount() {
	amount := tgju.Amount{Text: "1,864,000", Value: 1_864_000}

	fmt.Println(amount.Text)
	fmt.Println(amount.Rial())
	fmt.Println(amount.Toman())
	// Output:
	// 1,864,000
	// 1864000
	// 186400
}

// Markets can be resolved from a string, which is how a configuration file or a
// command line argument becomes a fetch.
func ExampleParseMarket() {
	for _, name := range []string{"gold", "FX", "سکه", "crypto"} {
		market, err := tgju.ParseMarket(name)
		if err != nil {
			fmt.Printf("%s: %v\n", name, err)
			continue
		}
		fmt.Printf("%s: %s (%s)\n", name, market, market.Label())
	}
	// Output:
	// gold: gold (طلا و نقره)
	// FX: currency (ارز)
	// سکه: coin (سکه)
	// crypto: tgju: unknown market: "crypto"
}
