package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"
)

// middleware wraps a handler with one cross cutting concern. The chain is built
// once in [New] and shared by every request.
type middleware func(http.Handler) http.Handler

// chain applies middlewares so that the first argument is the outermost, which
// is the order they are written in [New].
func chain(h http.Handler, mw ...middleware) http.Handler {
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}

// contextKey is unexported so nothing outside this package can collide with it.
type contextKey int

const (
	ctxRequestID contextKey = iota
	ctxLogger
	ctxRoute
)

// routeHolder carries the matched pattern back out to the access log.
//
// [http.ServeMux] records the pattern on a copy of the request, which the
// middleware wrapped around the mux never sees. A pointer placed on the context
// before dispatch and filled in by the handler bridges the gap; it is written
// and read by the one goroutine serving the request, so no lock is needed.
type routeHolder struct{ pattern string }

// recordRoute notes which pattern was matched. Registering a handler through it
// is what keeps the metrics labelled by route rather than by raw path.
func recordRoute(pattern string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if holder, ok := r.Context().Value(ctxRoute).(*routeHolder); ok {
			holder.pattern = pattern
		}
		h(w, r)
	}
}

// RequestIDFrom returns the identifier assigned to the request being served, or
// "" outside a request. Every response carries it in X-Request-Id, and every
// log line about the request mentions it.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(ctxRequestID).(string)
	return id
}

// loggerFrom returns the request scoped logger, falling back to the default.
func loggerFrom(r *http.Request) *slog.Logger {
	if l, ok := r.Context().Value(ctxLogger).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

// recoverer turns a panic in a handler into a 500 instead of a dropped
// connection, and logs the stack once.
func recoverer(logger *slog.Logger) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				// A client that hung up mid-response is not a bug worth a
				// stack, and the panic has to keep travelling: the server
				// itself is what interprets it.
				if err, ok := rec.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(rec)
				}
				logger.ErrorContext(r.Context(), "server: handler panicked",
					slog.Any("panic", rec),
					slog.String("stack", string(debug.Stack())),
					slog.String("request_id", RequestIDFrom(r.Context())))

				writeError(w, r, APIError{Code: CodeInternal, Message: "internal error"})
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// requestID adopts an incoming X-Request-Id when it looks sane and mints one
// otherwise, then puts it on the context, the response and the request logger.
func requestID(logger *slog.Logger) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := sanitiseRequestID(r.Header.Get("X-Request-Id"))
			if id == "" {
				id = newRequestID()
			}

			ctx := context.WithValue(r.Context(), ctxRequestID, id)
			ctx = context.WithValue(ctx, ctxLogger, logger.With(slog.String("request_id", id)))

			w.Header().Set("X-Request-Id", id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// sanitiseRequestID accepts a caller supplied identifier only when it cannot
// poison a log file or a response header.
func sanitiseRequestID(id string) string {
	if len(id) == 0 || len(id) > 64 {
		return ""
	}
	for _, r := range id {
		if r > 'z' || (!isAlphanumeric(r) && r != '-' && r != '_' && r != '.') {
			return ""
		}
	}
	return id
}

func isAlphanumeric(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func newRequestID() string {
	var buf [12]byte
	_, _ = rand.Read(buf[:])
	return hex.EncodeToString(buf[:])
}

// accessLog records one line per request once it has finished, and feeds the
// metrics registry.
func accessLog(logger *slog.Logger, m *metrics, now func() time.Time) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			holder := &routeHolder{}

			next.ServeHTTP(rec, r.WithContext(context.WithValue(r.Context(), ctxRoute, holder)))

			elapsed := now().Sub(start)
			route := holder.pattern
			if route == "" {
				route = "unmatched"
			}
			m.observe(route, r.Method, rec.status, elapsed)

			logger.InfoContext(r.Context(), "server: request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("route", route),
				slog.Int("status", rec.status),
				slog.Int("bytes", rec.written),
				slog.Duration("duration", elapsed),
				slog.String("request_id", RequestIDFrom(r.Context())))
		})
	}
}

// readOnly rejects anything that is not a read.
//
// The whole API is GET, so a POST is a mistake about the method rather than
// about the path, and 405 says so more usefully than the 404 a catch-all route
// would otherwise produce.
func readOnly() middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
			default:
				w.Header().Set("Allow", "GET, HEAD, OPTIONS")
				writeJSON(w, r, http.StatusMethodNotAllowed, APIError{
					Code:      CodeNotFound,
					Message:   "this API is read only; use GET",
					RequestID: RequestIDFrom(r.Context()),
				})
			}
		})
	}
}

// statusRecorder remembers what a handler wrote, for the access log.
type statusRecorder struct {
	http.ResponseWriter
	status  int
	written int
	sent    bool
}

func (s *statusRecorder) WriteHeader(status int) {
	if s.sent {
		return
	}
	s.sent, s.status = true, status
	s.ResponseWriter.WriteHeader(status)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	s.sent = true
	n, err := s.ResponseWriter.Write(b)
	s.written += n
	return n, err
}

// Unwrap lets http.ResponseController reach the underlying writer for
// flushing and hijacking.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// cors answers preflight requests and adds the headers a browser needs. The API
// is read only and public, so the policy is simple and credential free.
func cors(origins []string) middleware {
	allowAll := len(origins) == 1 && origins[0] == "*"
	allowed := make(map[string]bool, len(origins))
	for _, o := range origins {
		allowed[o] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			switch {
			case origin == "":
				// Not a browser request; nothing to negotiate.
			case allowAll:
				w.Header().Set("Access-Control-Allow-Origin", "*")
			case allowed[origin]:
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Add("Vary", "Origin")
			}

			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Request-Id")
				w.Header().Set("Access-Control-Max-Age", "86400")
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// rateLimiter is a per client token bucket.
//
// It is deliberately small: this service exists to be put in front of a site
// that would rather not be scraped, and the limiter is there to keep one
// enthusiastic caller from becoming everyone's problem. Anything more demanding
// belongs in a real gateway.
type rateLimiter struct {
	rate  float64 // tokens per second
	burst float64
	now   func() time.Time

	mu      sync.Mutex
	buckets map[string]*bucket
	swept   time.Time
}

type bucket struct {
	tokens float64
	seen   time.Time
}

func newRateLimiter(rate float64, burst int, now func() time.Time) *rateLimiter {
	return &rateLimiter{
		rate:    rate,
		burst:   float64(burst),
		now:     now,
		buckets: map[string]*bucket{},
		swept:   now(),
	}
}

// allow reports whether the client may make one more request.
func (l *rateLimiter) allow(client string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweep(now)

	b, ok := l.buckets[client]
	if !ok {
		b = &bucket{tokens: l.burst}
		l.buckets[client] = b
	} else {
		b.tokens = min(l.burst, b.tokens+now.Sub(b.seen).Seconds()*l.rate)
	}
	b.seen = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// sweepInterval is how often idle buckets are collected. Without it the map
// grows for the lifetime of the process, one entry per address ever seen.
const sweepInterval = 10 * time.Minute

func (l *rateLimiter) sweep(now time.Time) {
	if now.Sub(l.swept) < sweepInterval {
		return
	}
	l.swept = now
	for key, b := range l.buckets {
		if now.Sub(b.seen) > sweepInterval {
			delete(l.buckets, key)
		}
	}
}

func (l *rateLimiter) middleware() middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !l.allow(clientOf(r)) {
				w.Header().Set("Retry-After", strconv.Itoa(max(int(1/l.rate), 1)))
				writeError(w, r, APIError{Code: CodeRateLimited,
					Message: "too many requests; slow down or run your own instance"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientOf identifies the caller for rate limiting: the left-most address of
// X-Forwarded-For when the service sits behind a proxy, otherwise the peer.
//
// X-Forwarded-For is trivially forged, which is fine here — the limiter is a
// courtesy, not a security control — but it is the only way to tell two users
// behind one load balancer apart.
func clientOf(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if first, _, ok := strings.Cut(fwd, ","); ok {
			return strings.TrimSpace(first)
		}
		return strings.TrimSpace(fwd)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// timeout bounds a request. It is applied inside the access log so that a
// timed out request is still recorded.
func timeout(d time.Duration) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
