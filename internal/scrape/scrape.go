// Package scrape turns a tgju.org market page into plain rows of text.
//
// The package is deliberately free of domain vocabulary: it knows about tables,
// header cells and slugs, and nothing about currencies, gold or rials. Mapping
// a [Row] onto a domain type is the job of the caller, which keeps the fragile
// half of the system — the one that breaks when tgju restyles a page — small,
// pure and testable against saved fixtures.
package scrape

import (
	"errors"
	"io"
	"strings"

	"github.com/amiranmanesh/tgju-api-go/internal/dom"
	"github.com/amiranmanesh/tgju-api-go/internal/numfmt"
)

// ErrNoTables is returned when a page carries no market table at all. In
// practice that means tgju served an error page, a captcha or a redesign.
var ErrNoTables = errors.New("scrape: no market table found in the document")

// Row is a single line of a market table, with every cell kept as the site
// rendered it apart from digit normalisation and whitespace collapsing.
type Row struct {
	// Slug is the machine name tgju gives the instrument, e.g. "price_eur".
	Slug string
	// Title is the human readable name, e.g. "یورو".
	Title string
	// Price is the live price cell.
	Price string
	// Change is the raw change cell, e.g. "(0.32%) 6,050".
	Change string
	// Status is "low", "high" or "" — read from the class of the span inside
	// the change cell rather than from the sign of the number, because tgju
	// renders the amount unsigned.
	Status string
	// Low and High are the daily extremes.
	Low, High string
	// Time is the timestamp cell: a clock for actively traded instruments
	// ("11:49:45") and a date for stale ones ("24 مرداد").
	Time string
	// Profile is the href of the row's chart link, relative to the site root.
	Profile string
}

// Table is one market table together with the caption tgju puts in its first
// header cell, e.g. "قیمت طلا" or "حباب سکه".
type Table struct {
	Title string
	Rows  []Row
}

// Tables parses an HTML document and returns every market table it carries.
//
// It returns [ErrNoTables] when the document has none; a table whose body is
// empty is reported with no rows rather than dropped, so a caller can tell
// "tgju published an empty section" from "tgju changed its markup".
func Tables(r io.Reader) ([]Table, error) {
	root, err := dom.Parse(r)
	if err != nil {
		return nil, err
	}

	nodes := dom.FindAll(root, dom.TagClass("table", marketTableClass))
	if len(nodes) == 0 {
		return nil, ErrNoTables
	}

	tables := make([]Table, 0, len(nodes))
	for _, node := range nodes {
		tables = append(tables, parseTable(node))
	}
	return tables, nil
}

// marketTableClass is the class tgju puts on every table that holds prices. It
// has survived several redesigns of the site and is the single selector this
// package depends on.
const marketTableClass = "market-table"

// layout maps a logical column onto its index in a table body row. tgju keeps
// the same seven columns on every market page, but the order is read from the
// header rather than assumed, so a reshuffle upstream does not silently swap
// the daily low with the daily high.
type layout struct {
	price, change, low, high, time int
}

// defaultLayout is the historical column order, used when the header cells
// cannot be recognised.
var defaultLayout = layout{price: 0, change: 1, low: 2, high: 3, time: 4}

// headerKeywords maps the Persian header captions onto the layout fields. The
// captions are matched by substring, since tgju appends help icons and
// tooltips to them.
var headerKeywords = []struct {
	word string
	set  func(*layout, int)
}{
	{"قیمت زنده", func(l *layout, i int) { l.price = i }},
	{"نرخ زنده", func(l *layout, i int) { l.price = i }},
	{"تغییر", func(l *layout, i int) { l.change = i }},
	{"کمترین", func(l *layout, i int) { l.low = i }},
	{"بیشترین", func(l *layout, i int) { l.high = i }},
	{"زمان", func(l *layout, i int) { l.time = i }},
}

func parseTable(table *dom.Node) Table {
	title, cols := parseHead(table)

	var rows []Row
	for _, body := range dom.Children(table, dom.Tag("tbody")) {
		for _, tr := range dom.Children(body, dom.Tag("tr")) {
			if row, ok := parseRow(tr, cols); ok {
				rows = append(rows, row)
			}
		}
	}
	return Table{Title: title, Rows: rows}
}

// parseHead returns the table caption and the column layout. The caption is the
// first header cell, which tgju reuses as the section title ("قیمت طلا",
// "حباب سکه", or a generic "عنوان" on the currency page).
func parseHead(table *dom.Node) (string, layout) {
	head := dom.Find(table, dom.Tag("thead"))
	if head == nil {
		return "", defaultLayout
	}
	tr := dom.Find(head, dom.Tag("tr"))
	if tr == nil {
		return "", defaultLayout
	}

	cells := dom.Children(tr, dom.Tag("th"))
	if len(cells) == 0 {
		return "", defaultLayout
	}

	title := numfmt.Clean(dom.Text(cells[0]))

	// The body rows spend their first column on a th, so the header index of a
	// data column is one ahead of its index among the body's td cells.
	cols, matched := layout{-1, -1, -1, -1, -1}, 0
	for i, cell := range cells[1:] {
		text := dom.Text(cell)
		for _, kw := range headerKeywords {
			if strings.Contains(text, kw.word) {
				kw.set(&cols, i)
				matched++
				break
			}
		}
	}
	if matched < 3 {
		return title, defaultLayout
	}
	return title, cols
}

func parseRow(tr *dom.Node, cols layout) (Row, bool) {
	th := dom.Find(tr, dom.Tag("th"))
	cells := dom.Children(tr, dom.Tag("td"))
	if th == nil && len(cells) == 0 {
		return Row{}, false
	}

	row := Row{
		Slug:  dom.Attr(tr, "data-market-nameslug"),
		Title: numfmt.Clean(dom.Text(th)),
		Price: numfmt.Clean(dom.Attr(tr, "data-price")),
	}

	at := func(i int) *dom.Node {
		if i >= 0 && i < len(cells) {
			return cells[i]
		}
		return nil
	}

	if cell := at(cols.price); cell != nil {
		if text := numfmt.Clean(dom.Text(cell)); text != "" {
			row.Price = text
		}
	}
	if cell := at(cols.change); cell != nil {
		row.Change = numfmt.Clean(dom.Text(cell))
		row.Status = statusOf(cell)
	}
	row.Low = numfmt.Clean(dom.Text(at(cols.low)))
	row.High = numfmt.Clean(dom.Text(at(cols.high)))
	row.Time = numfmt.Clean(dom.Text(at(cols.time)))

	if len(cells) > 0 {
		if a := dom.Find(cells[len(cells)-1], dom.Tag("a")); a != nil {
			row.Profile = dom.Attr(a, "href")
		}
	}

	// tgju has no other place to put the slug, so a row without one — a
	// spacer, an advertisement, a table footer — is not a price.
	if row.Slug == "" {
		row.Slug = slugFromHref(row.Profile)
	}
	if row.Slug == "" || row.Title == "" {
		return Row{}, false
	}
	return row, true
}

// statusOf reads the direction of the daily change from the classes of the
// span inside the change cell. tgju writes class="low" on the currency and
// gold pages and class="type low" on some others, so every class token is
// considered.
func statusOf(cell *dom.Node) string {
	for _, span := range dom.FindAll(cell, dom.Tag("span")) {
		for _, class := range dom.Classes(span) {
			switch class {
			case "low", "high":
				return class
			}
		}
	}
	return ""
}

// slugFromHref extracts "price_eur" from "profile/price_eur".
func slugFromHref(href string) string {
	href = strings.TrimSuffix(strings.TrimSpace(href), "/")
	if href == "" {
		return ""
	}
	if i := strings.LastIndexByte(href, '/'); i >= 0 {
		href = href[i+1:]
	}
	if i := strings.IndexAny(href, "?#"); i >= 0 {
		href = href[:i]
	}
	return href
}
