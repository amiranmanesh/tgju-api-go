package tgju

import (
	"iter"
	"math"
	"strconv"
	"time"
)

// Status is the direction of an instrument's move since the previous close, as
// tgju itself classifies it. It is read from the markup rather than derived
// from the numbers, because the site renders the change unsigned.
type Status string

// The possible values of [Status].
const (
	// StatusUnknown means tgju published no direction for the row, which it
	// does for instruments that have not moved and for stale boards.
	StatusUnknown Status = ""
	// StatusLow means the price fell.
	StatusLow Status = "low"
	// StatusHigh means the price rose.
	StatusHigh Status = "high"
)

// String implements [fmt.Stringer].
func (s Status) String() string { return string(s) }

// Valid reports whether s is a direction tgju actually publishes.
func (s Status) Valid() bool { return s == StatusLow || s == StatusHigh }

// Amount is a price as tgju renders it together with its numeric value.
//
// Both halves are kept because they answer different questions: Text is what
// you show a Persian speaking user, Value is what you compare, sort and store.
// Parsing is done once, during scraping, so a caller never has to strip
// thousands separators itself.
type Amount struct {
	// Text is the site's own rendering, e.g. "1,864,000".
	Text string `json:"text"`
	// Value is Text parsed as a number. Currency, gold and coin prices are
	// quoted in Iranian rial.
	Value float64 `json:"value"`
}

// IsZero reports whether the amount carries no value at all, which is how an
// empty table cell arrives.
func (a Amount) IsZero() bool { return a.Text == "" && a.Value == 0 }

// String implements [fmt.Stringer] and returns the site's rendering, falling
// back to the numeric value when the cell was built from an attribute.
func (a Amount) String() string {
	if a.Text != "" {
		return a.Text
	}
	return strconv.FormatFloat(a.Value, 'f', -1, 64)
}

// Rial returns the value rounded to whole rials.
func (a Amount) Rial() int64 { return int64(math.Round(a.Value)) }

// Toman returns the value in toman, the unit Iranians actually quote prices
// in. One toman is ten rials.
func (a Amount) Toman() float64 { return a.Value / 10 }

// Change is the move of an instrument since the previous close.
type Change struct {
	// Status is the direction of the move.
	Status Status `json:"status"`
	// Percent is the size of the move in percent, always positive; read
	// Status for the sign.
	Percent float64 `json:"percent"`
	// Amount is the size of the move in rials, always positive.
	Amount Amount `json:"amount"`
}

// Signum returns +1 when the price rose, -1 when it fell and 0 when tgju
// published no direction. It is the bridge between the site's unsigned numbers
// and arithmetic that needs a sign.
func (c Change) Signum() int {
	switch c.Status {
	case StatusHigh:
		return 1
	case StatusLow:
		return -1
	default:
		return 0
	}
}

// Item is a single instrument on a board: a currency pair, a gold weight, a
// coin.
type Item struct {
	// Key is tgju's own identifier, e.g. "price_dollar_rl" or "geram18". It is
	// stable across page redesigns and is what you should store.
	Key string `json:"key"`
	// Title is the Persian name, e.g. "دلار".
	Title string `json:"title"`
	// Market is the board the item was read from.
	Market Market `json:"market"`
	// Category is the caption of the table the item sat in, e.g. "قیمت نقره".
	Category string `json:"category"`
	// Price is the live price.
	Price Amount `json:"price"`
	// Low and High are the extremes of the current trading day.
	Low  Amount `json:"low"`
	High Amount `json:"high"`
	// Change is the move since the previous close.
	Change Change `json:"change"`
	// Time is the timestamp tgju prints next to the row: a clock for actively
	// traded instruments ("11:49:45") and a Persian date for stale ones
	// ("24 مرداد"). It is passed through as text because the site gives no
	// year, no timezone and no consistent format.
	Time string `json:"time"`
	// ProfileURL points at the instrument's page on tgju.org.
	ProfileURL string `json:"profile_url,omitempty"`
}

// Spread returns the distance between the daily high and the daily low. It is
// zero when either extreme is missing.
func (i Item) Spread() float64 {
	if i.High.IsZero() || i.Low.IsZero() {
		return 0
	}
	return i.High.Value - i.Low.Value
}

// Category is one table of a board, named after the caption tgju puts in its
// first header cell.
type Category struct {
	// Title is the caption, e.g. "قیمت طلا" or "حباب سکه".
	Title string `json:"title"`
	// Items are the rows of the table, in the order the site published them.
	Items []Item `json:"items"`
}

// Snapshot is everything one board published at one moment.
//
// It is a value: copying it is cheap enough and it is safe to share between
// goroutines as long as nobody mutates the slices it points at.
type Snapshot struct {
	// Market is the board this snapshot came from.
	Market Market `json:"market"`
	// Source is the URL that was fetched.
	Source string `json:"source"`
	// FetchedAt is when the page was retrieved, in UTC.
	FetchedAt time.Time `json:"fetched_at"`
	// Categories are the tables of the board, in page order.
	Categories []Category `json:"categories"`
}

// Len returns the number of items across every category.
func (s Snapshot) Len() int {
	n := 0
	for _, c := range s.Categories {
		n += len(c.Items)
	}
	return n
}

// All iterates over every item of the snapshot in page order.
//
//	for item := range snap.All() {
//	    fmt.Println(item.Key, item.Price.Text)
//	}
func (s Snapshot) All() iter.Seq[Item] {
	return func(yield func(Item) bool) {
		for _, c := range s.Categories {
			for _, item := range c.Items {
				if !yield(item) {
					return
				}
			}
		}
	}
}

// Items flattens the snapshot into a freshly allocated slice. Prefer [All] when
// you only need to walk the items once.
func (s Snapshot) Items() []Item {
	out := make([]Item, 0, s.Len())
	for item := range s.All() {
		out = append(out, item)
	}
	return out
}

// Keys returns the key of every item, in page order.
func (s Snapshot) Keys() []string {
	out := make([]string, 0, s.Len())
	for item := range s.All() {
		out = append(out, item.Key)
	}
	return out
}

// Lookup returns the item with the given key.
func (s Snapshot) Lookup(key string) (Item, bool) {
	for item := range s.All() {
		if item.Key == key {
			return item, true
		}
	}
	return Item{}, false
}

// Category returns the category with the given title. The currency board
// publishes two tables under the same generic caption, so the first match wins.
func (s Snapshot) Category(title string) (Category, bool) {
	for _, c := range s.Categories {
		if c.Title == title {
			return c, true
		}
	}
	return Category{}, false
}

// IsEmpty reports whether the snapshot carries no items.
func (s Snapshot) IsEmpty() bool { return s.Len() == 0 }
