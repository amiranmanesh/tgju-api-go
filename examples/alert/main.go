// Command alert watches an instrument and prints a line when it crosses a
// threshold.
//
// It shows the pattern a long running program wants: one client, a ticker, and
// error handling that tells "tgju had a bad minute" apart from "tgju changed its
// markup and this program will never work again until it is updated".
//
//	go run ./examples/alert -key price_dollar_rl -above 190000
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	tgju "github.com/amiranmanesh/tgju-api-go"
)

func main() {
	var (
		key      = flag.String("key", "price_dollar_rl", "instrument to watch")
		above    = flag.Float64("above", 0, "alert when the price in toman rises above this")
		below    = flag.Float64("below", 0, "alert when the price in toman falls below this")
		interval = flag.Duration("interval", time.Minute, "how often to poll")
	)
	flag.Parse()

	if *above == 0 && *below == 0 {
		fmt.Fprintln(os.Stderr, "give at least one of -above or -below")
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := watch(ctx, *key, *above, *below, *interval); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "alert:", err)
		os.Exit(1)
	}
}

func watch(ctx context.Context, key string, above, below float64, interval time.Duration) error {
	// The cache is set to half the poll interval: it collapses the three market
	// fetches a lookup makes without ever serving a price older than one tick.
	client := tgju.New(tgju.WithCacheTTL(interval / 2))

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// consecutive counts transport failures in a row. One is noise; several in a
	// row is an outage worth reporting to whoever runs this.
	var consecutive int

	for {
		item, err := client.Item(ctx, key)
		switch {
		case err == nil:
			consecutive = 0
			report(item, above, below)

		case errors.Is(err, tgju.ErrParse), errors.Is(err, tgju.ErrNotFound):
			// Neither of these fixes itself. Stop rather than spin.
			return err

		case ctx.Err() != nil:
			return ctx.Err()

		default:
			consecutive++
			if consecutive >= 3 {
				fmt.Fprintf(os.Stderr, "%s  upstream has failed %d times in a row: %v\n",
					time.Now().Format(time.TimeOnly), consecutive, err)
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func report(item tgju.Item, above, below float64) {
	toman := item.Price.Toman()
	now := time.Now().Format(time.TimeOnly)

	switch {
	case above > 0 && toman > above:
		fmt.Printf("%s  ALERT  %s is %.0f toman, above %.0f\n", now, item.Title, toman, above)
	case below > 0 && toman < below:
		fmt.Printf("%s  ALERT  %s is %.0f toman, below %.0f\n", now, item.Title, toman, below)
	default:
		fmt.Printf("%s  %s %.0f toman (%s %.2f%%)\n", now, item.Title, toman, item.Change.Status, item.Change.Percent)
	}
}
