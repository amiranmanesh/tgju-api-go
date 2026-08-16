package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	tgju "github.com/amiranmanesh/tgju-api-go"
	"github.com/amiranmanesh/tgju-api-go/server"
)

// serve runs the HTTP API until the context ends, then drains in flight
// requests before returning.
func serve(ctx context.Context, args []string, stderr io.Writer) error {
	fs := newFlagSet("serve [flags]", stderr, "Start the HTTP API.")

	var cf clientFlags
	cf.register(fs)

	var (
		addr           = fs.String("addr", envString("ADDR", ":8080"), "address to listen on")
		corsOrigins    = fs.String("cors", envString("CORS", "*"), "comma separated allowed origins; empty disables CORS")
		rateLimit      = fs.Float64("rate-limit", envFloat("RATE_LIMIT", server.DefaultRateLimit), "requests per second per client; 0 disables the limiter")
		rateBurst      = fs.Int("rate-burst", envInt("RATE_BURST", server.DefaultRateBurst), "burst allowance for the rate limiter")
		requestTimeout = fs.Duration("request-timeout", envDuration("REQUEST_TIMEOUT", server.DefaultRequestTimeout), "deadline for one API request")
		metrics        = fs.Bool("metrics", envBool("METRICS", true), "serve Prometheus metrics on /metrics")
		shutdownGrace  = fs.Duration("shutdown-grace", envDuration("SHUTDOWN_GRACE", 15*time.Second), "how long to wait for in flight requests on shutdown")
	)

	if _, err := parse(fs, args); err != nil {
		return err
	}

	logger := cf.logger(stderr)
	client := cf.client(tgju.WithLogger(logger))

	api := server.New(client,
		server.WithLogger(logger),
		server.WithCORS(splitCSV(*corsOrigins)...),
		server.WithRateLimit(*rateLimit, *rateBurst),
		server.WithRequestTimeout(*requestTimeout),
		server.WithMetrics(*metrics),
	)

	httpServer := &http.Server{
		Addr:    *addr,
		Handler: api,
		// The read and write timeouts are the backstop for a slow client; the
		// per request deadline above is the one that bounds our own work.
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       2 * time.Minute,
		BaseContext:       func(net.Listener) context.Context { return ctx },
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}

	logger.Info("tgju: listening",
		slog.String("addr", *addr),
		slog.String("version", tgju.Version),
		slog.String("upstream", client.BaseURL()),
		slog.Duration("cache_ttl", client.CacheTTL()),
		slog.String("docs", "http://"+displayAddr(*addr)+"/docs"))

	errCh := make(chan error, 1)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("listen on %s: %w", *addr, err)
		}
		return nil

	case <-ctx.Done():
		logger.Info("tgju: shutting down", slog.Duration("grace", *shutdownGrace))

		// A fresh context: the one that was cancelled cannot also govern the
		// drain it triggered.
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), *shutdownGrace)
		defer cancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
		logger.Info("tgju: stopped")
		return nil
	}
}

// splitCSV turns "a,b" into []string{"a","b"}, and "" into nil so that an empty
// --cors disables the headers entirely.
func splitCSV(raw string) []string {
	var out []string
	for part := range strings.SplitSeq(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// displayAddr turns a listen address into something clickable in a terminal.
func displayAddr(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "localhost" + addr
	}
	return addr
}
