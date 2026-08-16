package tgju

import (
	"fmt"
	"slices"
	"strings"
)

// Market is one of the price pages tgju.org publishes. It is the only thing a
// caller has to name to fetch data, and it doubles as the path segment of the
// HTTP API exposed by the server package.
type Market string

// The supported markets.
//
// Every one of them is rendered by tgju with the same table markup, which is
// what makes a single scraper enough. Pages built by client side JavaScript —
// the crypto board, for instance — are deliberately absent: scraping them
// would need a browser, and a browser has no place in a library.
const (
	// Currency is the foreign exchange board, https://www.tgju.org/currency.
	Currency Market = "currency"
	// Gold is the gold, silver and mesghal board,
	// https://www.tgju.org/gold-chart.
	Gold Market = "gold"
	// Coin is the Bahar Azadi coin board, https://www.tgju.org/coin.
	Coin Market = "coin"
)

// source describes where a market lives on tgju.org.
type source struct {
	// path is appended to the base URL to reach the page.
	path string
	// label is the Persian name of the board, used in documentation and in
	// the CLI listing.
	label string
}

// sources is the registry every lookup goes through. Adding a market to the
// library is adding a line here plus a fixture under testdata.
var sources = map[Market]source{
	Currency: {path: "/currency", label: "ارز"},
	Gold:     {path: "/gold-chart", label: "طلا و نقره"},
	Coin:     {path: "/coin", label: "سکه"},
}

// Markets returns the supported markets in a stable order.
func Markets() []Market {
	out := make([]Market, 0, len(sources))
	for m := range sources {
		out = append(out, m)
	}
	slices.Sort(out)
	return out
}

// ParseMarket resolves a market name, case insensitively and tolerating the
// aliases that read naturally in a URL or on a command line.
func ParseMarket(s string) (Market, error) {
	switch key := strings.ToLower(strings.TrimSpace(s)); key {
	case "currency", "currencies", "fx", "ارز":
		return Currency, nil
	case "gold", "gold-chart", "silver", "طلا":
		return Gold, nil
	case "coin", "coins", "سکه":
		return Coin, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownMarket, s)
	}
}

// String implements [fmt.Stringer].
func (m Market) String() string { return string(m) }

// Valid reports whether the market is one this library knows how to fetch.
func (m Market) Valid() bool { _, ok := sources[m]; return ok }

// Label returns the Persian name of the board, or "" for an unknown market.
func (m Market) Label() string { return sources[m].label }

// Path returns the path of the market page relative to the site root, or ""
// for an unknown market.
func (m Market) Path() string { return sources[m].path }

// URL returns the absolute address of the market page on the public site.
func (m Market) URL() string {
	if !m.Valid() {
		return ""
	}
	return DefaultBaseURL + m.Path()
}
