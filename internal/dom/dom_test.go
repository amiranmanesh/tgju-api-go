package dom_test

import (
	"strings"
	"testing"

	"github.com/amiranmanesh/tgju-api-go/internal/dom"
)

const page = `<!doctype html>
<html><body>
  <div id="outer" class="wrap panel">
    <table class="data-table market-table" data-tab-id="1">
      <thead><tr><th>عنوان</th><th>قیمت زنده</th></tr></thead>
      <tbody>
        <tr data-market-nameslug="price_eur">
          <th><span class="mini-flag flag-eu"></span>یورو</th>
          <td class="nf">2,155,100</td>
          <td class="nf"><span class="high">(0.25%) 5,400</span></td>
        </tr>
      </tbody>
    </table>
    <table class="data-table"><tbody><tr><td>not a market</td></tr></tbody></table>
  </div>
</body></html>`

func parse(t *testing.T) *dom.Node {
	t.Helper()

	root, err := dom.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return root
}

func TestFind(t *testing.T) {
	t.Parallel()

	root := parse(t)

	table := dom.Find(root, dom.TagClass("table", "market-table"))
	if table == nil {
		t.Fatal("the market table was not found")
	}
	if got := dom.Attr(table, "data-tab-id"); got != "1" {
		t.Errorf("data-tab-id = %q, want 1", got)
	}
	if dom.Find(root, dom.Tag("form")) != nil {
		t.Error("Find invented a form that is not in the document")
	}
}

func TestFindAll(t *testing.T) {
	t.Parallel()

	root := parse(t)

	if got := len(dom.FindAll(root, dom.Tag("table"))); got != 2 {
		t.Errorf("found %d tables, want 2", got)
	}
	if got := len(dom.FindAll(root, dom.TagClass("table", "market-table"))); got != 1 {
		t.Errorf("found %d market tables, want 1", got)
	}
	if got := len(dom.FindAll(root, dom.TagClass("", "nf"))); got != 2 {
		t.Errorf("found %d cells with class nf, want 2", got)
	}
}

// TestFindAllDoesNotDescendIntoMatches keeps nested tables from being reported
// twice, which is what would happen if tgju ever wrapped one table in another.
func TestFindAllDoesNotDescendIntoMatches(t *testing.T) {
	t.Parallel()

	const nested = `<div><table class="market-table"><tbody><tr><td>
	  <table class="market-table"><tbody><tr><td>inner</td></tr></tbody></table>
	</td></tr></tbody></table></div>`

	root, err := dom.Parse(strings.NewReader(nested))
	if err != nil {
		t.Fatal(err)
	}
	if got := len(dom.FindAll(root, dom.TagClass("table", "market-table"))); got != 1 {
		t.Errorf("found %d tables, want only the outer one", got)
	}
}

func TestChildren(t *testing.T) {
	t.Parallel()

	root := parse(t)
	tr := dom.Find(root, dom.Tag("tbody"))

	rows := dom.Children(tr, dom.Tag("tr"))
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if got := len(dom.Children(rows[0], dom.Tag("td"))); got != 2 {
		t.Errorf("got %d direct td children, want 2", got)
	}
	if dom.Children(nil, dom.Tag("td")) != nil {
		t.Error("Children(nil) must be nil")
	}
}

func TestAttrAndClasses(t *testing.T) {
	t.Parallel()

	root := parse(t)
	outer := dom.Find(root, dom.Tag("div"))

	if got := dom.Attr(outer, "id"); got != "outer" {
		t.Errorf("id = %q", got)
	}
	if got := dom.Attr(outer, "missing"); got != "" {
		t.Errorf("a missing attribute returned %q", got)
	}
	if got := dom.Attr(nil, "id"); got != "" {
		t.Errorf("Attr(nil) returned %q", got)
	}
	if !dom.HasClass(outer, "panel") || !dom.HasClass(outer, "wrap") {
		t.Error("HasClass missed a class token")
	}
	if dom.HasClass(outer, "wra") {
		t.Error("HasClass matched a prefix of a class token")
	}
	if dom.HasClass(nil, "wrap") {
		t.Error("HasClass(nil) must be false")
	}
	if got := len(dom.Classes(outer)); got != 2 {
		t.Errorf("got %d classes, want 2", got)
	}
}

func TestText(t *testing.T) {
	t.Parallel()

	root := parse(t)

	th := dom.Find(dom.Find(root, dom.Tag("tbody")), dom.Tag("th"))
	if got := dom.Text(th); got != "یورو" {
		t.Errorf("Text = %q, want یورو (the empty flag span must contribute nothing)", got)
	}
	if got := dom.Text(nil); got != "" {
		t.Errorf("Text(nil) = %q", got)
	}

	head := dom.Find(root, dom.Tag("thead"))
	if got := dom.Text(head); got != "عنوان قیمت زنده" {
		t.Errorf("Text collapsed whitespace into %q", got)
	}
}

// TestTagFallsBackToTheName covers an element name the html package has no atom
// for, which is how a custom element or a typo in the markup arrives.
func TestTagFallsBackToTheName(t *testing.T) {
	t.Parallel()

	root, err := dom.Parse(strings.NewReader(`<div><price-widget id="x">1</price-widget></div>`))
	if err != nil {
		t.Fatal(err)
	}
	node := dom.Find(root, dom.Tag("price-widget"))
	if node == nil {
		t.Fatal("the custom element was not found")
	}
	if got := dom.Attr(node, "id"); got != "x" {
		t.Errorf("id = %q", got)
	}
}
