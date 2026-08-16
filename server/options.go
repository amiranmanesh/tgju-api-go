package server

import (
	"log/slog"
	"time"
)

// Defaults applied by [New].
const (
	// DefaultRequestTimeout bounds one API request, upstream fetch included.
	DefaultRequestTimeout = 30 * time.Second
	// DefaultRateLimit is the per client request rate. Zero would be no limit;
	// a scraper in front of a site that does not want to be scraped should
	// have one by default.
	DefaultRateLimit = 20
	// DefaultRateBurst is how many requests a client may make back to back
	// before the rate applies.
	DefaultRateBurst = 40
)

// config holds everything a [Server] was built with.
type config struct {
	logger         *slog.Logger
	corsOrigins    []string
	requestTimeout time.Duration
	rateLimit      float64
	rateBurst      int
	metrics        bool
	now            func() time.Time
}

// Option configures a [Server].
type Option func(*config)

func newConfig(opts ...Option) config {
	cfg := config{
		logger:         slog.Default(),
		corsOrigins:    []string{"*"},
		requestTimeout: DefaultRequestTimeout,
		rateLimit:      DefaultRateLimit,
		rateBurst:      DefaultRateBurst,
		metrics:        true,
		now:            time.Now,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return cfg
}

// WithLogger sets the structured logger used for access logs and errors.
func WithLogger(l *slog.Logger) Option {
	return func(c *config) {
		if l != nil {
			c.logger = l
		}
	}
}

// WithCORS replaces the list of allowed origins. Pass "*" to allow every
// origin, or no argument at all to disable CORS headers entirely.
//
// Credentials are never allowed: this API is public data and has no session to
// protect, and "*" with credentials is rejected by browsers anyway.
func WithCORS(origins ...string) Option {
	return func(c *config) { c.corsOrigins = origins }
}

// WithRequestTimeout bounds one API request. Zero or less disables the
// deadline, which is only sensible when something upstream imposes one.
func WithRequestTimeout(d time.Duration) Option {
	return func(c *config) { c.requestTimeout = d }
}

// WithRateLimit sets the per client rate in requests per second and the burst
// allowance. A rate of zero or less turns the limiter off.
//
// Clients are identified by the address the connection came from, or by the
// left-most entry of X-Forwarded-For when the server sits behind a proxy.
func WithRateLimit(perSecond float64, burst int) Option {
	return func(c *config) {
		c.rateLimit = perSecond
		if burst > 0 {
			c.rateBurst = burst
		}
	}
}

// WithMetrics enables or disables the /metrics endpoint.
func WithMetrics(enabled bool) Option {
	return func(c *config) { c.metrics = enabled }
}

// WithClock replaces the source of time, for tests.
func WithClock(now func() time.Time) Option {
	return func(c *config) {
		if now != nil {
			c.now = now
		}
	}
}
