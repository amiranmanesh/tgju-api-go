package server

import (
	"net/http"

	tgju "github.com/amiranmanesh/tgju-api-go"
)

// This file keeps the wire format of BlackIQ/tgju-api, the FastAPI service this
// project reimplements, alive under /api/price/*.
//
// It exists so an existing consumer can change one hostname and be done. New
// clients should use /v1, which carries the parsed numbers, the change, the
// timestamp and the market each row belongs to; the legacy shape has none of
// that, and its fields are all strings.

// legacyItem is the schema of the original API: every value a string, and no
// distinction between "tgju published nothing" and "the value is empty".
type legacyItem struct {
	Title     string `json:"title"`
	Price     string `json:"price"`
	Key       string `json:"key"`
	Status    string `json:"status"`
	LowPrice  string `json:"low_price"`
	HighPrice string `json:"high_price"`
}

// legacyCategory is the grouped shape the original API used for gold.
type legacyCategory struct {
	Title  string       `json:"title"`
	Prices []legacyItem `json:"prices"`
}

// legacyShape selects between the two response layouts the original API used:
// a flat array for currency, an array of categories for gold.
type legacyShape int

const (
	// legacyFlat drops the category grouping, as /api/price/currency did.
	legacyFlat legacyShape = iota
	// legacyGrouped keeps it, as /api/price/gold did.
	legacyGrouped
)

func (s *Server) handleLegacy(market tgju.Market, shape legacyShape) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snap, err := s.client.Fetch(r.Context(), market)
		if err != nil {
			writeError(w, r, err)
			return
		}

		s.cacheControl(w)
		writeJSON(w, r, http.StatusOK, legacyBody(snap, shape))
	}
}

func legacyBody(snap tgju.Snapshot, shape legacyShape) any {
	if shape == legacyFlat {
		out := make([]legacyItem, 0, snap.Len())
		for item := range snap.All() {
			out = append(out, toLegacyItem(item))
		}
		return out
	}

	out := make([]legacyCategory, 0, len(snap.Categories))
	for _, category := range snap.Categories {
		prices := make([]legacyItem, 0, len(category.Items))
		for _, item := range category.Items {
			prices = append(prices, toLegacyItem(item))
		}
		out = append(out, legacyCategory{Title: category.Title, Prices: prices})
	}
	return out
}

func toLegacyItem(item tgju.Item) legacyItem {
	return legacyItem{
		Title:     item.Title,
		Price:     item.Price.Text,
		Key:       item.Key,
		Status:    string(item.Change.Status),
		LowPrice:  item.Low.Text,
		HighPrice: item.High.Text,
	}
}
