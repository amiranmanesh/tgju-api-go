// Command basic prints today's gold and currency prices.
//
// It is the smallest useful program you can write against this library: build a
// client, fetch a board, read the rows.
//
//	go run ./examples/basic
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	tgju "github.com/amiranmanesh/tgju-api-go"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := tgju.New()

	if err := printBoard(ctx, client, tgju.Currency, "price_dollar_rl", "price_eur", "price_aed"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println()
	if err := printBoard(ctx, client, tgju.Gold, "geram18", "geram24", "silver_925"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func printBoard(ctx context.Context, client *tgju.Client, market tgju.Market, keys ...string) error {
	snap, err := client.Fetch(ctx, market)
	if err != nil {
		return err
	}

	fmt.Printf("%s (%s) — %d instruments, fetched %s\n",
		market.Label(), market, snap.Len(), snap.FetchedAt.Format(time.TimeOnly))

	for _, key := range keys {
		item, ok := snap.Lookup(key)
		if !ok {
			log.Printf("tgju no longer publishes %q", key)
			continue
		}

		fmt.Printf("  %-12s %-14s %14s toman  %s %.2f%%\n",
			item.Key, item.Title, format(item.Price.Toman()), arrow(item.Change.Status), item.Change.Percent)
	}
	return nil
}

func arrow(status tgju.Status) string {
	switch status {
	case tgju.StatusHigh:
		return "▲"
	case tgju.StatusLow:
		return "▼"
	default:
		return "·"
	}
}

// format adds thousands separators, which the library deliberately does not do:
// how a number is shown is a decision for the program showing it.
func format(v float64) string {
	s := fmt.Sprintf("%.0f", v)

	var out []byte
	for i, digit := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, digit)
	}
	return string(out)
}
