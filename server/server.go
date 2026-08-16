// Package server exposes a [tgju.Client] over HTTP.
//
// A [Server] is an [net/http.Handler], which is the whole point: the same code
// runs as the standalone binary in cmd/tgju and as a subtree of an existing
// service. Nothing here starts a listener or reads the environment; that is the
// caller's job, and cmd/tgju shows one way to do it.
//
//	client := tgju.New()
//	api := server.New(client)
//
//	mux := http.NewServeMux()
//	mux.Handle("/tgju/", http.StripPrefix("/tgju", api))
//
// The API is read only. Every endpoint answers GET, returns JSON, and is safe
// to cache for the lifetime of the client's snapshot TTL, which the responses
// advertise in Cache-Control.
package server

import (
	"net/http"
	"strconv"
	"time"

	tgju "github.com/amiranmanesh/tgju-api-go"
)

// Server serves the price boards over HTTP.
//
// Build it with [New]; the zero value is not usable. It is safe for concurrent
// use, and holds no state of its own beyond metrics and the rate limiter.
type Server struct {
	client  *tgju.Client
	cfg     config
	metrics *metrics
	handler http.Handler
	routes  []Route
}

// Route describes one endpoint. [Server.Routes] returns them all, which is how
// the documentation page and the root redirect stay in step with the router.
type Route struct {
	// Method is the HTTP method, always GET on this API.
	Method string `json:"method"`
	// Pattern is the path, with {placeholders} for the variable segments.
	Pattern string `json:"pattern"`
	// Summary is a one line description.
	Summary string `json:"summary"`
}

// New returns a server reading from client.
//
// The client is not owned by the server: its timeout, retry policy and cache
// TTL are configured where it is built, and the same client may back several
// servers.
func New(client *tgju.Client, opts ...Option) *Server {
	if client == nil {
		client = tgju.New()
	}
	cfg := newConfig(opts...)

	s := &Server{
		client:  client,
		cfg:     cfg,
		metrics: newMetrics(cfg.metrics, cfg.now),
	}
	s.handler = s.build()
	return s
}

// ServeHTTP implements [net/http.Handler].
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.handler.ServeHTTP(w, r) }

// Routes returns the endpoints the server answers, in the order they are
// documented.
func (s *Server) Routes() []Route { return s.routes }

// Client returns the client the server reads from.
func (s *Server) Client() *tgju.Client { return s.client }

// build assembles the router and wraps it in the middleware chain. The order
// below is the order a request travels through:
//
//	recoverer -> requestID -> accessLog -> cors -> rateLimit -> timeout -> mux
//
// The access log sits above CORS and the limiter so that a rejected preflight
// or a throttled client still appears in the log and in the metrics.
func (s *Server) build() http.Handler {
	mux := http.NewServeMux()

	// handle registers an endpoint and documents it in the same breath, so
	// Routes(), the documentation page and the router cannot drift apart.
	handle := func(pattern, summary string, h http.HandlerFunc) {
		s.routes = append(s.routes, Route{Method: http.MethodGet, Pattern: pattern, Summary: summary})
		mux.HandleFunc("GET "+pattern, recordRoute(pattern, h))
	}

	handle("/v1/markets", "List the supported markets", s.handleMarkets)
	handle("/v1/markets/{market}", "Full snapshot of one market, grouped into categories", s.handleMarket)
	handle("/v1/markets/{market}/items", "Flat list of one market's instruments", s.handleMarketItems)
	handle("/v1/markets/{market}/items/{key}", "One instrument of one market", s.handleMarketItem)
	handle("/v1/items/{key}", "One instrument, looked up across every market", s.handleItem)
	handle("/v1/snapshot", "Every market in one response", s.handleSnapshot)

	handle("/api/price/currency", "Currency prices in the shape of the original Python API",
		s.handleLegacy(tgju.Currency, legacyFlat))
	handle("/api/price/gold", "Gold prices in the shape of the original Python API",
		s.handleLegacy(tgju.Gold, legacyGrouped))
	handle("/api/price/coin", "Coin prices in the shape of the original Python API",
		s.handleLegacy(tgju.Coin, legacyGrouped))

	handle("/healthz", "Liveness probe", s.handleHealth)
	handle("/readyz", "Readiness probe; fetches from tgju.org", s.handleReady)
	handle("/openapi.yaml", "The OpenAPI description of this API", s.handleOpenAPI)
	handle("/docs", "This documentation", s.handleDocs)

	if s.cfg.metrics {
		handle("/metrics", "Prometheus metrics", s.metrics.handler(s.cfg.now))
	}

	mux.HandleFunc("GET /{$}", recordRoute("/", s.handleRoot))
	mux.HandleFunc("/", notFound)

	mw := []middleware{
		recoverer(s.cfg.logger),
		requestID(s.cfg.logger),
		accessLog(s.cfg.logger, s.metrics, s.cfg.now),
		readOnly(),
	}
	if len(s.cfg.corsOrigins) > 0 {
		mw = append(mw, cors(s.cfg.corsOrigins))
	}
	if s.cfg.rateLimit > 0 {
		mw = append(mw, newRateLimiter(s.cfg.rateLimit, s.cfg.rateBurst, s.cfg.now).middleware())
	}
	if s.cfg.requestTimeout > 0 {
		mw = append(mw, timeout(s.cfg.requestTimeout))
	}

	return chain(mux, mw...)
}

// cacheControl advertises how long a response stays fresh, which mirrors the
// client's snapshot TTL. A CDN or a browser then repeats the cache the client
// already keeps, one layer further out.
func (s *Server) cacheControl(w http.ResponseWriter) {
	ttl := s.client.CacheTTL()
	if ttl <= 0 {
		w.Header().Set("Cache-Control", "no-store")
		return
	}
	w.Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(int(ttl/time.Second)))
}
