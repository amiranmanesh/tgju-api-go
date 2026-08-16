// Package fixture serves saved tgju.org pages to the test suites.
//
// The pages are real markup, trimmed to a handful of rows per table so the
// repository stays small, with the giant tooltip attributes stripped. Keeping
// one copy behind an embed means the parser tests, the client tests and the
// server tests all agree on what tgju looks like, and that refreshing the
// fixtures is a single step.
//
// It is an internal package, so importing testing here costs no user of the
// library anything.
package fixture

import (
	"embed"
	"net/http"
	"net/http/httptest"
	"testing"
)

//go:embed testdata/*.html
var files embed.FS

// Paths maps the tgju.org path of each supported board onto its fixture.
var Paths = map[string]string{
	"/currency":   "testdata/currency.html",
	"/gold-chart": "testdata/gold.html",
	"/coin":       "testdata/coin.html",
}

// Page returns the saved page for a tgju.org path such as "/currency".
func Page(tb testing.TB, path string) []byte {
	tb.Helper()

	name, ok := Paths[path]
	if !ok {
		tb.Fatalf("fixture: no saved page for %q", path)
	}
	body, err := files.ReadFile(name)
	if err != nil {
		tb.Fatalf("fixture: read %s: %v", name, err)
	}
	return body
}

// Server starts an HTTP server that answers the tgju.org board paths with the
// saved pages and everything else with 404. It is shut down when the test ends.
//
//	client := tgju.New(tgju.WithBaseURL(fixture.Server(t).URL))
func Server(tb testing.TB) *httptest.Server {
	tb.Helper()
	return ServerFunc(tb, nil)
}

// ServerFunc is [Server] with a hook that runs before every response. Use it to
// count requests, to assert on headers, or to fail a request on purpose:
//
//	srv := fixture.ServerFunc(t, func(w http.ResponseWriter, r *http.Request) bool {
//	    w.WriteHeader(http.StatusTooManyRequests)
//	    return false // the fixture is not written
//	})
//
// The hook returns false to take over the response entirely.
func ServerFunc(tb testing.TB, hook func(http.ResponseWriter, *http.Request) bool) *httptest.Server {
	tb.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hook != nil && !hook(w, r) {
			return
		}
		name, ok := Paths[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		body, err := files.ReadFile(name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(body)
	}))
	tb.Cleanup(srv.Close)

	return srv
}
