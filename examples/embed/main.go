// Command embed shows the library and the HTTP API living inside somebody
// else's service.
//
// Two things are happening at once, and they are the point of this repository:
//
//   - /shop/quote calls the library directly, in process, with no HTTP hop —
//     this is the "use it as a module" half;
//   - /prices/* mounts the ready made API under a prefix — this is the
//     "expose it" half.
//
// One client backs both, so the cache is shared: a quote and an API request
// arriving in the same window cost one fetch from tgju.org between them.
//
//	go run ./examples/embed
//	curl localhost:8080/shop/quote?amount=250
//	curl localhost:8080/prices/v1/markets/gold
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	tgju "github.com/amiranmanesh/tgju-api-go"
	"github.com/amiranmanesh/tgju-api-go/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// One client for the whole process: it owns the connection pool and the
	// snapshot cache, and both halves of this service benefit from both.
	prices := tgju.New(
		tgju.WithCacheTTL(30*time.Second),
		tgju.WithLogger(logger),
	)

	shop := &shop{prices: prices, logger: logger}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /shop/quote", shop.quote)
	mux.Handle("/prices/", http.StripPrefix("/prices", server.New(prices, server.WithLogger(logger))))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "try /shop/quote?amount=250 or /prices/v1/markets/gold\n")
	})

	httpServer := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdown)
	}()

	logger.Info("listening", slog.String("addr", httpServer.Addr))
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

// shop is the imaginary business logic: it prices jewellery in toman from the
// live 18 carat gold rate.
type shop struct {
	prices *tgju.Client
	logger *slog.Logger
}

// quoteResponse is what a customer sees. Note what it does not contain: any
// hint that the number came from scraping a web page.
type quoteResponse struct {
	Grams     float64   `json:"grams"`
	RateToman float64   `json:"rate_toman_per_gram"`
	Toman     float64   `json:"total_toman"`
	QuotedAt  time.Time `json:"quoted_at"`
}

func (s *shop) quote(w http.ResponseWriter, r *http.Request) {
	grams, err := strconv.ParseFloat(r.URL.Query().Get("amount"), 64)
	if err != nil || grams <= 0 {
		http.Error(w, `{"error":"amount must be a positive number of grams"}`, http.StatusBadRequest)
		return
	}

	// This is the whole integration: one call, one struct back.
	gold, err := s.prices.Item(r.Context(), "geram18", tgju.Gold)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "could not price the quote", slog.String("error", err.Error()))

		// A stale price is worse than no price for a shop, so the failure is
		// surfaced rather than papered over.
		http.Error(w, `{"error":"prices are temporarily unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	// tgju quotes 18 carat gold per gram, in rial.
	rate := gold.Price.Toman()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(quoteResponse{
		Grams:     grams,
		RateToman: rate,
		Toman:     rate * grams,
		QuotedAt:  time.Now().UTC(),
	})
}
