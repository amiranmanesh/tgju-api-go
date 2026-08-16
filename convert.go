package tgju

import (
	"strings"
	"time"

	"github.com/amiranmanesh/tgju-api-go/internal/numfmt"
	"github.com/amiranmanesh/tgju-api-go/internal/scrape"
)

// snapshotFrom maps the scraper's text rows onto the domain model. It is the
// single place where "cell four of the table" becomes "the daily high", which
// is why it is a plain function: no network, no clock, no configuration, and
// therefore trivially testable against a fixture.
func snapshotFrom(m Market, baseURL, source string, at time.Time, tables []scrape.Table) Snapshot {
	snap := Snapshot{
		Market:     m,
		Source:     source,
		FetchedAt:  at.UTC(),
		Categories: make([]Category, 0, len(tables)),
	}

	for _, table := range tables {
		if len(table.Rows) == 0 {
			continue
		}
		category := Category{Title: table.Title, Items: make([]Item, 0, len(table.Rows))}
		for _, row := range table.Rows {
			category.Items = append(category.Items, itemFrom(m, table.Title, baseURL, row))
		}
		snap.Categories = append(snap.Categories, category)
	}
	return snap
}

func itemFrom(m Market, category, baseURL string, row scrape.Row) Item {
	percent, changeAmount := numfmt.Change(row.Change)

	return Item{
		Key:      row.Slug,
		Title:    row.Title,
		Market:   m,
		Category: category,
		Price:    amountOf(row.Price),
		Low:      amountOf(row.Low),
		High:     amountOf(row.High),
		Change: Change{
			Status:  statusOf(row.Status),
			Percent: percent,
			Amount:  amountOf(changeAmount),
		},
		Time:       row.Time,
		ProfileURL: absoluteURL(baseURL, row.Profile),
	}
}

// amountOf pairs a rendered cell with its numeric value. A cell that carries no
// digits — an empty column, a dash — yields the zero [Amount] rather than a
// text with a meaningless zero next to it.
func amountOf(text string) Amount {
	text = strings.TrimSpace(text)
	value, ok := numfmt.Value(text)
	if !ok {
		return Amount{}
	}
	return Amount{Text: text, Value: value}
}

func statusOf(s string) Status {
	switch Status(s) {
	case StatusLow:
		return StatusLow
	case StatusHigh:
		return StatusHigh
	default:
		return StatusUnknown
	}
}

// absoluteURL resolves the site relative hrefs tgju emits ("profile/geram18")
// against the base URL. Absolute hrefs are returned untouched.
func absoluteURL(baseURL, href string) string {
	href = strings.TrimSpace(href)
	switch {
	case href == "":
		return ""
	case strings.HasPrefix(href, "http://"), strings.HasPrefix(href, "https://"):
		return href
	case strings.HasPrefix(href, "//"):
		return "https:" + href
	case strings.HasPrefix(href, "/"):
		return baseURL + href
	default:
		return baseURL + "/" + href
	}
}
