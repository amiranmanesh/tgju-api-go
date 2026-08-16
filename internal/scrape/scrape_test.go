package scrape_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/amiranmanesh/tgju-api-go/internal/fixture"
	"github.com/amiranmanesh/tgju-api-go/internal/scrape"
)

func TestTablesOnSavedPages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path           string
		wantTables     int
		wantFirstTitle string
		wantFirstSlug  string
	}{
		{"/currency", 2, "عنوان", "price_dollar_rl"},
		{"/gold-chart", 5, "قیمت طلا", "geram18"},
		{"/coin", 5, "قیمت نقدی", "sekee"},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()

			tables, err := scrape.Tables(bytes.NewReader(fixture.Page(t, tc.path)))
			if err != nil {
				t.Fatalf("Tables: %v", err)
			}
			if len(tables) != tc.wantTables {
				t.Fatalf("got %d tables, want %d", len(tables), tc.wantTables)
			}
			if tables[0].Title != tc.wantFirstTitle {
				t.Errorf("first table title = %q, want %q", tables[0].Title, tc.wantFirstTitle)
			}
			if len(tables[0].Rows) == 0 {
				t.Fatal("first table has no rows")
			}
			if got := tables[0].Rows[0].Slug; got != tc.wantFirstSlug {
				t.Errorf("first slug = %q, want %q", got, tc.wantFirstSlug)
			}

			for _, table := range tables {
				for _, row := range table.Rows {
					if row.Slug == "" {
						t.Errorf("%s: row %q has no slug", table.Title, row.Title)
					}
					if row.Title == "" {
						t.Errorf("%s: row %q has no title", table.Title, row.Slug)
					}
					if row.Price == "" {
						t.Errorf("%s: row %q has no price", table.Title, row.Slug)
					}
				}
			}
		})
	}
}

// TestRowFields pins every cell of one well known row, so a change in the
// column mapping cannot pass unnoticed.
func TestRowFields(t *testing.T) {
	t.Parallel()

	tables, err := scrape.Tables(bytes.NewReader(fixture.Page(t, "/currency")))
	if err != nil {
		t.Fatalf("Tables: %v", err)
	}

	row, ok := findRow(tables, "price_dollar_rl")
	if !ok {
		t.Fatal("price_dollar_rl not found")
	}

	want := scrape.Row{
		Slug:    "price_dollar_rl",
		Title:   "دلار",
		Price:   "1,864,000",
		Change:  "(0.32%) 6,050",
		Status:  "low",
		Low:     "1,860,800",
		High:    "1,869,100",
		Time:    "11:49:45",
		Profile: "profile/price_dollar_rl",
	}
	if row != want {
		t.Errorf("row =\n %+v\nwant\n %+v", row, want)
	}
}

// TestStalePageRow covers the "طلا در بورس" table, whose rows carry a Persian
// date instead of a clock and an empty class on the change span.
func TestStalePageRow(t *testing.T) {
	t.Parallel()

	tables, err := scrape.Tables(bytes.NewReader(fixture.Page(t, "/gold-chart")))
	if err != nil {
		t.Fatalf("Tables: %v", err)
	}

	row, ok := findRow(tables, "ime_fund_atash")
	if !ok {
		t.Fatal("ime_fund_atash not found")
	}
	if row.Status != "" {
		t.Errorf("status = %q, want empty for an unmoved instrument", row.Status)
	}
	if !strings.Contains(row.Time, "مرداد") {
		t.Errorf("time = %q, want a Persian date", row.Time)
	}
	if row.Change != "(0%) 0" {
		t.Errorf("change = %q, want %q", row.Change, "(0%) 0")
	}
}

func TestTablesRejectsPagesWithoutMarketTables(t *testing.T) {
	t.Parallel()

	tests := []struct{ name, body string }{
		{"empty document", ""},
		{"error page", `<html><body><h1>خطا</h1></body></html>`},
		{"an unrelated table", `<html><body><table class="data-table"><tr><td>x</td></tr></table></body></html>`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := scrape.Tables(strings.NewReader(tc.body)); !errors.Is(err, scrape.ErrNoTables) {
				t.Fatalf("err = %v, want ErrNoTables", err)
			}
		})
	}
}

// TestColumnsAreReadFromTheHeader proves the parser follows the header rather
// than fixed positions: the daily low and high are swapped in the markup and the
// values must follow their captions.
func TestColumnsAreReadFromTheHeader(t *testing.T) {
	t.Parallel()

	const page = `<table class="data-table market-table">
	  <thead><tr>
	    <th>عنوان</th><th>قیمت زنده</th><th>تغییر</th><th>بیشترین</th><th>کمترین</th><th>زمان</th><th></th>
	  </tr></thead>
	  <tbody><tr data-market-nameslug="price_eur">
	    <th>یورو</th>
	    <td class="nf">2,155,100</td>
	    <td class="nf"><span class="high">(0.25%) 5,400</span></td>
	    <td>2,200,000</td>
	    <td>2,100,000</td>
	    <td>11:49:45</td>
	    <td><a href="profile/price_eur"></a></td>
	  </tr></tbody>
	</table>`

	tables, err := scrape.Tables(strings.NewReader(page))
	if err != nil {
		t.Fatalf("Tables: %v", err)
	}
	row := tables[0].Rows[0]
	if row.High != "2,200,000" {
		t.Errorf("high = %q, want 2,200,000", row.High)
	}
	if row.Low != "2,100,000" {
		t.Errorf("low = %q, want 2,100,000", row.Low)
	}
	if row.Status != "high" {
		t.Errorf("status = %q, want high", row.Status)
	}
}

// TestFallsBackToPositionsWithoutAHeader covers a table whose captions this
// parser cannot recognise, where the historical column order is the best guess
// available.
func TestFallsBackToPositionsWithoutAHeader(t *testing.T) {
	t.Parallel()

	const page = `<table class="data-table market-table">
	  <thead><tr><th>x</th><th>a</th><th>b</th><th>c</th><th>d</th><th>e</th></tr></thead>
	  <tbody><tr data-market-nameslug="sekee">
	    <th>سکه</th><td class="nf">100</td><td class="nf">(1%) 2</td><td>90</td><td>110</td><td>11:00:00</td>
	  </tr></tbody>
	</table>`

	tables, err := scrape.Tables(strings.NewReader(page))
	if err != nil {
		t.Fatalf("Tables: %v", err)
	}
	row := tables[0].Rows[0]
	if row.Price != "100" || row.Low != "90" || row.High != "110" {
		t.Errorf("positional fallback gave price=%q low=%q high=%q", row.Price, row.Low, row.High)
	}
}

func TestRowsWithoutASlugAreSkipped(t *testing.T) {
	t.Parallel()

	const page = `<table class="data-table market-table">
	  <tbody>
	    <tr><td colspan="7">تبلیغات</td></tr>
	    <tr data-market-nameslug="price_eur"><th>یورو</th><td class="nf">1</td></tr>
	    <tr><th>بدون شناسه</th><td class="nf">2</td></tr>
	  </tbody>
	</table>`

	tables, err := scrape.Tables(strings.NewReader(page))
	if err != nil {
		t.Fatalf("Tables: %v", err)
	}
	if got := len(tables[0].Rows); got != 1 {
		t.Fatalf("got %d rows, want 1", got)
	}
}

// TestSlugFallsBackToTheProfileLink covers rows where tgju drops the data
// attribute but still links to the instrument page.
func TestSlugFallsBackToTheProfileLink(t *testing.T) {
	t.Parallel()

	const page = `<table class="data-table market-table">
	  <tbody><tr>
	    <th>طلای ۱۸ عیار</th><td class="nf">1</td><td class="nf">2</td><td>3</td><td>4</td><td>5</td>
	    <td><a href="/profile/geram18?tab=chart"></a></td>
	  </tr></tbody>
	</table>`

	tables, err := scrape.Tables(strings.NewReader(page))
	if err != nil {
		t.Fatalf("Tables: %v", err)
	}
	if got := tables[0].Rows[0].Slug; got != "geram18" {
		t.Errorf("slug = %q, want geram18", got)
	}
}

func findRow(tables []scrape.Table, slug string) (scrape.Row, bool) {
	for _, table := range tables {
		for _, row := range table.Rows {
			if row.Slug == slug {
				return row, true
			}
		}
	}
	return scrape.Row{}, false
}

func BenchmarkTables(b *testing.B) {
	page := fixture.Page(b, "/gold-chart")
	b.ReportAllocs()
	b.SetBytes(int64(len(page)))

	for b.Loop() {
		if _, err := scrape.Tables(bytes.NewReader(page)); err != nil {
			b.Fatal(err)
		}
	}
}
