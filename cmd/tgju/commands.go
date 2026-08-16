package main

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	tgju "github.com/amiranmanesh/tgju-api-go"
)

// buildVersion is stamped at link time by the release build:
//
//	go build -ldflags "-X main.buildVersion=$(git describe --tags)"
//
// It falls back to the library's own constant, and then to whatever the module
// system recorded, so a `go install` of a tagged version still reports it.
var buildVersion string

// get prints one board.
func get(ctx context.Context, args []string, stdout io.Writer) error {
	fs := newFlagSet("get <market> [flags]", stdout,
		"Print one board. Markets: "+marketNames()+".")

	var cf clientFlags
	cf.register(fs)

	var (
		formatFlag = fs.String("format", envString("FORMAT", string(formatTable)), "table, json or csv")
		unitFlag   = fs.String("unit", envString("UNIT", string(unitRaw)), "rial, toman or raw")
		keys       = fs.String("keys", "", "comma separated instrument keys to keep")
		category   = fs.String("category", "", "keep only the category with this exact title")
	)

	positional, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(positional) != 1 {
		return usageErrorf("get needs exactly one market; try one of %s", marketNames())
	}

	market, err := tgju.ParseMarket(positional[0])
	if err != nil {
		return err
	}
	out, unitOut, err := parseOutput(*formatFlag, *unitFlag)
	if err != nil {
		return err
	}

	snap, err := cf.client().Fetch(ctx, market)
	if err != nil {
		return err
	}

	snap.Categories = filter(snap.Categories, *category, splitCSV(*keys))
	if snap.IsEmpty() {
		return fmt.Errorf("%w: no instrument on %s matched the filters", tgju.ErrNotFound, market)
	}

	return writeSnapshot(stdout, snap, out, unitOut)
}

// item prints one instrument, looked up across every board.
func item(ctx context.Context, args []string, stdout io.Writer) error {
	fs := newFlagSet("item <key> [flags]", stdout,
		"Print one instrument, e.g. price_dollar_rl, geram18 or sekee.")

	var cf clientFlags
	cf.register(fs)

	var (
		formatFlag = fs.String("format", envString("FORMAT", string(formatTable)), "table, json or csv")
		unitFlag   = fs.String("unit", envString("UNIT", string(unitRaw)), "rial, toman or raw")
		marketFlag = fs.String("market", "", "restrict the search to one board")
	)

	positional, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(positional) != 1 {
		return usageErrorf("item needs exactly one instrument key")
	}
	out, unitOut, err := parseOutput(*formatFlag, *unitFlag)
	if err != nil {
		return err
	}

	var markets []tgju.Market
	if *marketFlag != "" {
		market, err := tgju.ParseMarket(*marketFlag)
		if err != nil {
			return err
		}
		markets = append(markets, market)
	}

	found, err := cf.client().Item(ctx, positional[0], markets...)
	if err != nil {
		return err
	}
	return writeItem(stdout, found, out, unitOut)
}

// watch follows one instrument until the context ends, printing a line whenever
// the price changes.
func watch(ctx context.Context, args []string, stdout io.Writer) error {
	fs := newFlagSet("watch <key> [flags]", stdout,
		"Follow one instrument, printing a line whenever its price changes.")

	var cf clientFlags
	cf.register(fs)

	var (
		interval = fs.Duration("interval", envDuration("INTERVAL", time.Minute), "how often to poll")
		unitFlag = fs.String("unit", envString("UNIT", string(unitRaw)), "rial, toman or raw")
		all      = fs.Bool("all", false, "print every poll, not only the changes")
	)

	positional, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(positional) != 1 {
		return usageErrorf("watch needs exactly one instrument key")
	}
	unitOut, err := parseUnit(*unitFlag)
	if err != nil {
		return err
	}
	if *interval < time.Second {
		return usageErrorf("--interval must be at least one second; tgju updates every few seconds")
	}

	key := positional[0]
	// Polling faster than the cache would just re-read memory, so the cache is
	// pinned to the poll interval regardless of what the shared flag says.
	client := cf.client(tgju.WithCacheTTL(*interval / 2))

	var previous string
	for {
		found, err := client.Item(ctx, key)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			fmt.Fprintf(stdout, "%s  %s\n", time.Now().Format("15:04:05"), err)
		} else if current := unitOut.render(found.Price); *all || current != previous {
			fmt.Fprintf(stdout, "%s  %s %s  %s %s%%  %s\n",
				time.Now().Format("15:04:05"),
				found.Title, current,
				arrow(found.Change.Status), trimFloat(found.Change.Percent),
				found.Time)
			previous = current
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(*interval):
		}
	}
}

// markets lists the supported boards.
func markets(args []string, stdout io.Writer) error {
	fs := newFlagSet("markets", stdout, "List the boards this build can read.")
	if _, err := parse(fs, args); err != nil {
		return err
	}

	rows := [][]string{{"MARKET", "NAME", "SOURCE"}}
	for _, m := range tgju.Markets() {
		rows = append(rows, []string{string(m), m.Label(), m.URL()})
	}
	return writeAligned(stdout, rows)
}

// version prints the build information.
func version(args []string, stdout io.Writer) error {
	fs := newFlagSet("version", stdout, "Print the version and build information.")
	if _, err := parse(fs, args); err != nil {
		return err
	}

	rows := [][]string{
		{"version", resolveVersion()},
		{"library", tgju.Version},
		{"go", runtime.Version()},
		{"platform", runtime.GOOS + "/" + runtime.GOARCH},
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				rows = append(rows, []string{"commit", setting.Value})
			case "vcs.time":
				rows = append(rows, []string{"built", setting.Value})
			}
		}
	}
	return writeAligned(stdout, rows)
}

func resolveVersion() string {
	if buildVersion != "" {
		return buildVersion
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return tgju.Version
}

// filter applies the --category and --keys flags. It mirrors the query
// parameters of the HTTP API so that the two interfaces answer alike.
func filter(categories []tgju.Category, category string, keys []string) []tgju.Category {
	if category == "" && len(keys) == 0 {
		return categories
	}

	wanted := make(map[string]bool, len(keys))
	for _, key := range keys {
		wanted[key] = true
	}

	out := make([]tgju.Category, 0, len(categories))
	for _, c := range categories {
		if category != "" && c.Title != category {
			continue
		}
		if len(wanted) == 0 {
			out = append(out, c)
			continue
		}

		kept := make([]tgju.Item, 0, len(c.Items))
		for _, i := range c.Items {
			if wanted[i.Key] {
				kept = append(kept, i)
			}
		}
		if len(kept) > 0 {
			out = append(out, tgju.Category{Title: c.Title, Items: kept})
		}
	}
	return out
}

func parseOutput(formatFlag, unitFlag string) (format, unit, error) {
	out, err := parseFormat(formatFlag)
	if err != nil {
		return "", "", err
	}
	u, err := parseUnit(unitFlag)
	if err != nil {
		return "", "", err
	}
	return out, u, nil
}

func marketNames() string {
	names := make([]string, 0, len(tgju.Markets()))
	for _, m := range tgju.Markets() {
		names = append(names, string(m))
	}
	return strings.Join(names, ", ")
}

func trimFloat(v float64) string {
	return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".")
}
