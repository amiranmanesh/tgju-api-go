// Command fixtures refreshes the saved tgju.org pages the tests parse.
//
// Fixtures rot: tgju restyles its pages, and a suite that only ever sees a
// snapshot from the day the parser was written stops proving anything. This
// tool refetches each board, trims the pages down to the market tables and
// writes them back under internal/fixture/testdata.
//
//	make fixtures
//	go test ./...       # the diff in the fixtures is the review
//
// Trimming is what keeps the repository small: a live tgju page is around a
// megabyte, almost all of it inline CSS and per-row tooltip attributes that the
// parser ignores. What survives is the markup the scraper actually reads.
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tgju "github.com/amiranmanesh/tgju-api-go"
)

// rowsPerTable is how many rows of each table are kept. Enough to exercise the
// parser, few enough to read in a diff.
const rowsPerTable = 6

// outputDir is relative to the module root, which is where `go run ./...`
// starts.
const outputDir = "internal/fixture/testdata"

// The patterns below strip what the parser never looks at. They are applied to
// the extracted tables only, so nothing outside a market table can be affected.
var (
	tableRE   = regexp.MustCompile(`(?s)<table[^>]*\bmarket-table\b[^>]*>.*?</table>`)
	tooltipRE = regexp.MustCompile(`\sdata-title="[^"]*"`)
	onclickRE = regexp.MustCompile(`\sonclick="[^"]*"`)
	cfRE      = regexp.MustCompile(`\sdata-cf-modified-[^=]*=""`)
	tbodyRE   = regexp.MustCompile(`(?s)<tbody>(.*?)</tbody>`)
	rowRE     = regexp.MustCompile(`(?s)<tr[^>]*>.*?</tr>`)
)

// files maps each market onto the fixture it produces.
var files = map[tgju.Market]string{
	tgju.Currency: "currency.html",
	tgju.Gold:     "gold.html",
	tgju.Coin:     "coin.html",
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fixtures:", err)
		os.Exit(1)
	}
}

func run() error {
	if _, err := os.Stat(outputDir); err != nil {
		return fmt.Errorf("run this from the module root: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	for _, market := range tgju.Markets() {
		name, ok := files[market]
		if !ok {
			return fmt.Errorf("market %q has no fixture name; add one to this tool", market)
		}

		page, err := fetch(ctx, market.URL())
		if err != nil {
			return fmt.Errorf("%s: %w", market, err)
		}

		trimmed, tables := trim(page)
		if tables == 0 {
			return fmt.Errorf("%s: no market table in the page; the selector may have changed", market)
		}

		path := filepath.Join(outputDir, name)
		if err := os.WriteFile(path, []byte(trimmed), 0o644); err != nil {
			return err
		}
		fmt.Printf("%-10s %-16s %d tables, %d bytes\n", market, name, tables, len(trimmed))
	}

	fmt.Println("\nrun `go test ./...` and review the diff before committing")
	return nil
}

func fetch(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", tgju.DefaultUserAgent)
	req.Header.Set("Accept-Language", "fa-IR,fa;q=0.9")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, tgju.DefaultMaxBodyBytes))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// trim extracts the market tables and reduces each to a few rows.
func trim(page string) (string, int) {
	tables := tableRE.FindAllString(page, -1)
	if len(tables) == 0 {
		return "", 0
	}

	cleaned := make([]string, 0, len(tables))
	for _, table := range tables {
		table = tooltipRE.ReplaceAllString(table, "")
		table = onclickRE.ReplaceAllString(table, "")
		table = cfRE.ReplaceAllString(table, "")
		table = tbodyRE.ReplaceAllStringFunc(table, keepFirstRows)
		cleaned = append(cleaned, table)
	}

	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"fa\" dir=\"rtl\">\n")
	b.WriteString("<head><meta charset=\"utf-8\"><title>fixture</title></head>\n<body>\n<div class=\"wrap\">\n")
	b.WriteString(strings.Join(cleaned, "\n"))
	b.WriteString("\n</div>\n</body>\n</html>\n")

	return b.String(), len(cleaned)
}

func keepFirstRows(body string) string {
	rows := rowRE.FindAllString(body, -1)
	return "<tbody>" + strings.Join(rows[:min(len(rows), rowsPerTable)], "") + "</tbody>"
}
