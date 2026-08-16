package tgju_test

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	tgju "github.com/amiranmanesh/tgju-api-go"
)

func demoSnapshot() tgju.Snapshot {
	return tgju.Snapshot{
		Market:    tgju.Gold,
		Source:    "https://www.tgju.org/gold-chart",
		FetchedAt: time.Date(2026, 8, 16, 8, 20, 0, 0, time.UTC),
		Categories: []tgju.Category{
			{Title: "قیمت طلا", Items: []tgju.Item{
				{Key: "geram18", Title: "طلای 18 عیار", Price: tgju.Amount{Text: "190,542,000", Value: 190_542_000},
					Low:  tgju.Amount{Text: "190,394,000", Value: 190_394_000},
					High: tgju.Amount{Text: "190,692,000", Value: 190_692_000},
					Change: tgju.Change{Status: tgju.StatusLow, Percent: 0.13,
						Amount: tgju.Amount{Text: "252,000", Value: 252_000}}},
				{Key: "geram24", Title: "طلای 24 عیار", Price: tgju.Amount{Text: "254,056,000", Value: 254_056_000}},
			}},
			{Title: "قیمت نقره", Items: []tgju.Item{
				{Key: "silver_925", Title: "نقره 925", Price: tgju.Amount{Text: "3,765,900", Value: 3_765_900},
					Change: tgju.Change{Status: tgju.StatusHigh}},
			}},
		},
	}
}

func TestSnapshotLenAndItems(t *testing.T) {
	t.Parallel()

	snap := demoSnapshot()
	if got := snap.Len(); got != 3 {
		t.Errorf("Len() = %d, want 3", got)
	}
	if got := len(snap.Items()); got != 3 {
		t.Errorf("len(Items()) = %d, want 3", got)
	}
	want := []string{"geram18", "geram24", "silver_925"}
	if got := snap.Keys(); !slices.Equal(got, want) {
		t.Errorf("Keys() = %v, want %v", got, want)
	}
	if snap.IsEmpty() {
		t.Error("IsEmpty() = true for a populated snapshot")
	}
	if !(tgju.Snapshot{}).IsEmpty() {
		t.Error("IsEmpty() = false for the zero snapshot")
	}
}

func TestSnapshotAllStopsEarly(t *testing.T) {
	t.Parallel()

	seen := 0
	for range demoSnapshot().All() {
		seen++
		if seen == 2 {
			break
		}
	}
	if seen != 2 {
		t.Errorf("visited %d items after breaking, want 2", seen)
	}
}

func TestSnapshotLookup(t *testing.T) {
	t.Parallel()

	snap := demoSnapshot()

	item, ok := snap.Lookup("silver_925")
	if !ok {
		t.Fatal("silver_925 not found")
	}
	if item.Title != "نقره 925" {
		t.Errorf("title = %q", item.Title)
	}
	if _, ok := snap.Lookup("price_eur"); ok {
		t.Error("Lookup found a key the snapshot does not carry")
	}
}

func TestSnapshotCategory(t *testing.T) {
	t.Parallel()

	snap := demoSnapshot()

	category, ok := snap.Category("قیمت نقره")
	if !ok {
		t.Fatal("category not found")
	}
	if len(category.Items) != 1 {
		t.Errorf("got %d items, want 1", len(category.Items))
	}
	if _, ok := snap.Category("سکه"); ok {
		t.Error("Category found a title the snapshot does not carry")
	}
}

func TestAmount(t *testing.T) {
	t.Parallel()

	a := tgju.Amount{Text: "1,864,000", Value: 1_864_000}
	if got := a.String(); got != "1,864,000" {
		t.Errorf("String() = %q", got)
	}
	if got := a.Rial(); got != 1_864_000 {
		t.Errorf("Rial() = %d", got)
	}
	if got := a.Toman(); got != 186_400 {
		t.Errorf("Toman() = %v", got)
	}
	if a.IsZero() {
		t.Error("IsZero() = true for a populated amount")
	}

	if !(tgju.Amount{}).IsZero() {
		t.Error("IsZero() = false for the zero amount")
	}
	if got := (tgju.Amount{Value: 12.5}).String(); got != "12.5" {
		t.Errorf("String() without text = %q, want 12.5", got)
	}
	if got := (tgju.Amount{Value: 12.6}).Rial(); got != 13 {
		t.Errorf("Rial() = %d, want the value rounded to 13", got)
	}
}

func TestChangeSignum(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status tgju.Status
		want   int
	}{
		{tgju.StatusHigh, 1},
		{tgju.StatusLow, -1},
		{tgju.StatusUnknown, 0},
	}
	for _, tc := range tests {
		if got := (tgju.Change{Status: tc.status}).Signum(); got != tc.want {
			t.Errorf("Signum(%q) = %d, want %d", tc.status, got, tc.want)
		}
	}
}

func TestStatus(t *testing.T) {
	t.Parallel()

	if !tgju.StatusLow.Valid() || !tgju.StatusHigh.Valid() {
		t.Error("low and high must be valid statuses")
	}
	if tgju.StatusUnknown.Valid() {
		t.Error("the unknown status must not be valid")
	}
	if got := tgju.StatusHigh.String(); got != "high" {
		t.Errorf("String() = %q", got)
	}
}

func TestItemSpread(t *testing.T) {
	t.Parallel()

	item := tgju.Item{
		Low:  tgju.Amount{Text: "100", Value: 100},
		High: tgju.Amount{Text: "150", Value: 150},
	}
	if got := item.Spread(); got != 50 {
		t.Errorf("Spread() = %v, want 50", got)
	}
	if got := (tgju.Item{High: item.High}).Spread(); got != 0 {
		t.Errorf("Spread() without a low = %v, want 0", got)
	}
}

// TestSnapshotJSON pins the wire format, since it is what the HTTP API serves
// and what callers store.
func TestSnapshotJSON(t *testing.T) {
	t.Parallel()

	blob, err := json.Marshal(demoSnapshot().Categories[1].Items[0])
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	const want = `{"key":"silver_925","title":"نقره 925","market":"","category":"",` +
		`"price":{"text":"3,765,900","value":3765900},` +
		`"low":{"text":"","value":0},"high":{"text":"","value":0},` +
		`"change":{"status":"high","percent":0,"amount":{"text":"","value":0}},` +
		`"time":""}`

	if got := string(blob); got != want {
		t.Errorf("JSON =\n %s\nwant\n %s", got, want)
	}

	var back tgju.Item
	if err := json.Unmarshal(blob, &back); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if back.Key != "silver_925" || back.Price.Value != 3_765_900 {
		t.Errorf("round trip lost data: %+v", back)
	}
}
