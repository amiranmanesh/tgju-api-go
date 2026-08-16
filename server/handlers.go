package server

import (
	"net/http"
	"strings"

	tgju "github.com/amiranmanesh/tgju-api-go"
)

// MarketInfo describes one supported board. It is what GET /v1/markets returns,
// and it is enough for a client to discover the rest of the API without
// hardcoding market names.
type MarketInfo struct {
	// Name is the identifier used in paths, e.g. "gold".
	Name tgju.Market `json:"name"`
	// Label is the Persian name of the board.
	Label string `json:"label"`
	// Source is the page on tgju.org the data is read from.
	Source string `json:"source"`
	// Endpoint is the path on this API that serves the board.
	Endpoint string `json:"endpoint"`
}

func (s *Server) handleMarkets(w http.ResponseWriter, r *http.Request) {
	markets := tgju.Markets()
	out := make([]MarketInfo, 0, len(markets))
	for _, m := range markets {
		out = append(out, MarketInfo{
			Name:     m,
			Label:    m.Label(),
			Source:   s.client.BaseURL() + m.Path(),
			Endpoint: "/v1/markets/" + string(m),
		})
	}

	w.Header().Set("Cache-Control", "public, max-age=3600")
	writeJSON(w, r, http.StatusOK, map[string]any{"markets": out})
}

func (s *Server) handleMarket(w http.ResponseWriter, r *http.Request) {
	snap, err := s.snapshot(r)
	if err != nil {
		writeError(w, r, err)
		return
	}

	snap.Categories = filterCategories(snap.Categories, r)

	s.cacheControl(w)
	writeJSON(w, r, http.StatusOK, snap)
}

func (s *Server) handleMarketItems(w http.ResponseWriter, r *http.Request) {
	snap, err := s.snapshot(r)
	if err != nil {
		writeError(w, r, err)
		return
	}

	snap.Categories = filterCategories(snap.Categories, r)
	items := snap.Items()

	s.cacheControl(w)
	writeJSON(w, r, http.StatusOK, map[string]any{
		"market":     snap.Market,
		"fetched_at": snap.FetchedAt,
		"count":      len(items),
		"items":      items,
	})
}

func (s *Server) handleMarketItem(w http.ResponseWriter, r *http.Request) {
	snap, err := s.snapshot(r)
	if err != nil {
		writeError(w, r, err)
		return
	}

	key := r.PathValue("key")
	item, ok := snap.Lookup(key)
	if !ok {
		writeError(w, r, APIError{Code: CodeNotFound,
			Message: "market " + string(snap.Market) + " publishes no instrument called " + key})
		return
	}

	s.cacheControl(w)
	writeJSON(w, r, http.StatusOK, item)
}

func (s *Server) handleItem(w http.ResponseWriter, r *http.Request) {
	item, err := s.client.Item(r.Context(), r.PathValue("key"))
	if err != nil {
		writeError(w, r, err)
		return
	}

	s.cacheControl(w)
	writeJSON(w, r, http.StatusOK, item)
}

func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	all, err := s.client.FetchAll(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}

	// A map keyed by market keeps the response self describing, and the client
	// already guarantees every requested market is present.
	markets := make(map[string]tgju.Snapshot, len(all))
	for m, snap := range all {
		markets[string(m)] = snap
	}

	s.cacheControl(w)
	writeJSON(w, r, http.StatusOK, map[string]any{
		"fetched_at": s.cfg.now().UTC(),
		"markets":    markets,
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, r, http.StatusOK, map[string]any{
		"status":  "ok",
		"version": tgju.Version,
	})
}

// handleReady reports whether the service can actually do its job, which means
// asking tgju.org. It is separate from liveness on purpose: an orchestrator
// should stop sending traffic to an instance that cannot reach upstream, but it
// should not restart it, because restarting fixes nothing.
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	snap, err := s.client.Currency(r.Context())
	if err != nil {
		status, code := classify(err)
		writeJSON(w, r, max(status, http.StatusServiceUnavailable), map[string]any{
			"status": "unavailable",
			"reason": code,
		})
		return
	}

	writeJSON(w, r, http.StatusOK, map[string]any{
		"status":     "ok",
		"version":    tgju.Version,
		"upstream":   s.client.BaseURL(),
		"items":      snap.Len(),
		"fetched_at": snap.FetchedAt,
	})
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/docs", http.StatusFound)
}

// snapshot resolves the {market} path segment and fetches its board.
func (s *Server) snapshot(r *http.Request) (tgju.Snapshot, error) {
	market, err := tgju.ParseMarket(r.PathValue("market"))
	if err != nil {
		return tgju.Snapshot{}, err
	}
	return s.client.Fetch(r.Context(), market)
}

// filterCategories applies the ?category= and ?keys= query parameters.
//
// Filtering happens here rather than in the client because it is a presentation
// concern: the snapshot in the cache stays whole, and two callers asking for
// different slices still share one fetch.
func filterCategories(categories []tgju.Category, r *http.Request) []tgju.Category {
	query := r.URL.Query()
	category := strings.TrimSpace(query.Get("category"))
	keys := splitKeys(query.Get("keys"))

	if category == "" && len(keys) == 0 {
		return categories
	}

	out := make([]tgju.Category, 0, len(categories))
	for _, c := range categories {
		if category != "" && c.Title != category {
			continue
		}
		if len(keys) == 0 {
			out = append(out, c)
			continue
		}

		kept := make([]tgju.Item, 0, len(c.Items))
		for _, item := range c.Items {
			if keys[item.Key] {
				kept = append(kept, item)
			}
		}
		if len(kept) > 0 {
			out = append(out, tgju.Category{Title: c.Title, Items: kept})
		}
	}
	return out
}

// splitKeys parses a comma separated ?keys= parameter into a set.
func splitKeys(raw string) map[string]bool {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	out := map[string]bool{}
	for key := range strings.SplitSeq(raw, ",") {
		if key = strings.TrimSpace(key); key != "" {
			out[key] = true
		}
	}
	return out
}
