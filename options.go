package tgju

import (
	"log/slog"
	"net/http"
	"time"
)

// Defaults applied by [New] when the caller sets nothing.
const (
	// DefaultBaseURL is the public site. Override it with [WithBaseURL] to
	// point the client at a mirror, a caching proxy or a test server.
	DefaultBaseURL = "https://www.tgju.org"
	// DefaultTimeout bounds one page fetch, retries included.
	DefaultTimeout = 20 * time.Second
	// DefaultMaxBodyBytes caps how much of a response is read. tgju pages are
	// around one megabyte; the cap exists so a broken or hostile upstream
	// cannot exhaust the memory of a long lived service.
	DefaultMaxBodyBytes int64 = 16 << 20
	// DefaultCacheTTL is how long a snapshot is served from memory before the
	// page is fetched again. tgju updates prices every few seconds, so a short
	// window keeps the data fresh while collapsing a burst of API requests
	// into one outgoing request.
	DefaultCacheTTL = 30 * time.Second
)

// DefaultUserAgent is sent with every request. tgju serves an error page to
// clients that do not look like browsers, so the default impersonates one while
// still naming this library, which is the honest compromise: an operator
// reading their logs can tell what is calling them.
const DefaultUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 " +
	"(KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36 tgju-api-go/" + Version

// Doer is the subset of [http.Client] this package needs. Supply your own to
// plug in tracing, connection pooling policy, a proxy or a stub.
type Doer interface {
	// Do executes an HTTP request and returns its response.
	Do(req *http.Request) (*http.Response, error)
}

// RetryPolicy controls how transport failures are retried. Fetching a price
// board is idempotent, so retrying is always safe.
type RetryPolicy struct {
	// MaxAttempts is the total number of attempts including the first one.
	// Values below one disable retrying.
	MaxAttempts int
	// Backoff is the pause before the second attempt. It doubles after each
	// further failure.
	Backoff time.Duration
	// MaxBackoff caps the pause. Zero means uncapped.
	MaxBackoff time.Duration
}

// DefaultRetry retries twice with a short, doubling pause. Two extra attempts
// cover the connection resets tgju hands out under load without turning a
// genuine outage into a minute of blocked goroutines.
var DefaultRetry = RetryPolicy{MaxAttempts: 3, Backoff: 300 * time.Millisecond, MaxBackoff: 2 * time.Second}

// config holds everything a [Client] was built with. It is unexported so the
// option set can grow without breaking callers.
type config struct {
	http        Doer
	baseURL     string
	timeout     time.Duration
	userAgent   string
	header      http.Header
	retry       RetryPolicy
	cacheTTL    time.Duration
	maxBodySize int64
	logger      *slog.Logger
	now         func() time.Time
}

// Option configures a [Client]. Options are applied in order, so a later one
// wins.
type Option func(*config)

// newConfig applies opts on top of the defaults.
func newConfig(opts ...Option) config {
	cfg := config{
		baseURL:     DefaultBaseURL,
		timeout:     DefaultTimeout,
		userAgent:   DefaultUserAgent,
		header:      http.Header{},
		retry:       DefaultRetry,
		cacheTTL:    DefaultCacheTTL,
		maxBodySize: DefaultMaxBodyBytes,
		logger:      slog.New(slog.DiscardHandler),
		now:         time.Now,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if cfg.http == nil {
		cfg.http = &http.Client{Timeout: cfg.timeout}
	}
	return cfg
}

// WithHTTPClient replaces the HTTP client used for outgoing requests.
//
// The client keeps its own per call deadline, so a [http.Client] passed here
// does not need a Timeout of its own.
func WithHTTPClient(d Doer) Option {
	return func(c *config) {
		if d != nil {
			c.http = d
		}
	}
}

// WithBaseURL points the client at another host. The trailing slash is
// optional. It exists for mirrors, corporate proxies and, above all, tests.
func WithBaseURL(u string) Option {
	return func(c *config) {
		if u != "" {
			c.baseURL = trimTrailingSlash(u)
		}
	}
}

// WithTimeout bounds one call to [Client.Fetch], retries and backoff included.
// Zero or less restores [DefaultTimeout].
func WithTimeout(d time.Duration) Option {
	return func(c *config) {
		if d <= 0 {
			d = DefaultTimeout
		}
		c.timeout = d
	}
}

// WithUserAgent overrides the User-Agent header. An empty value is ignored;
// tgju answers requests without one with an error page.
func WithUserAgent(ua string) Option {
	return func(c *config) {
		if ua != "" {
			c.userAgent = ua
		}
	}
}

// WithHeader adds a header to every outgoing request. Call it repeatedly to set
// several; a repeated name replaces the previous value.
func WithHeader(name, value string) Option {
	return func(c *config) {
		if name != "" {
			c.header.Set(name, value)
		}
	}
}

// WithRetry replaces the retry policy. Pass RetryPolicy{} to disable retrying.
func WithRetry(p RetryPolicy) Option {
	return func(c *config) { c.retry = p }
}

// WithCacheTTL sets how long a fetched snapshot is reused. Zero disables the
// cache, and with it the collapsing of concurrent fetches for the same market.
//
// Leave the cache on when the client backs an HTTP API: it is the difference
// between one request to tgju per window and one per caller.
func WithCacheTTL(d time.Duration) Option {
	return func(c *config) {
		if d < 0 {
			d = 0
		}
		c.cacheTTL = d
	}
}

// WithMaxBodyBytes caps how much of a response is read. Zero or less restores
// [DefaultMaxBodyBytes].
func WithMaxBodyBytes(n int64) Option {
	return func(c *config) {
		if n <= 0 {
			n = DefaultMaxBodyBytes
		}
		c.maxBodySize = n
	}
}

// WithLogger sends request and cache events to a [slog.Logger] at debug level,
// and upstream failures at warn level. The default logger discards everything.
func WithLogger(l *slog.Logger) Option {
	return func(c *config) {
		if l != nil {
			c.logger = l
		}
	}
}

// WithClock replaces the source of time. Tests use it to drive cache expiry
// without sleeping.
func WithClock(now func() time.Time) Option {
	return func(c *config) {
		if now != nil {
			c.now = now
		}
	}
}

func trimTrailingSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
