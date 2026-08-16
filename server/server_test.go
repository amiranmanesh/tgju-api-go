package server_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tgju "github.com/amiranmanesh/tgju-api-go"
	"github.com/amiranmanesh/tgju-api-go/internal/fixture"
	"github.com/amiranmanesh/tgju-api-go/server"
)

// newAPI wires a server onto the saved tgju pages and returns a live instance.
func newAPI(t *testing.T, opts ...server.Option) *httptest.Server {
	t.Helper()

	upstream := fixture.Server(t)
	client := tgju.New(tgju.WithBaseURL(upstream.URL), tgju.WithCacheTTL(time.Minute))

	opts = append([]server.Option{
		server.WithLogger(slog.New(slog.DiscardHandler)),
		server.WithRateLimit(0, 0),
	}, opts...)

	api := httptest.NewServer(server.New(client, opts...))
	t.Cleanup(api.Close)

	return api
}

// get performs a request and decodes the JSON body into v, which may be nil.
func get(t *testing.T, api *httptest.Server, path string, v any) *http.Response {
	t.Helper()

	resp, err := api.Client().Get(api.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if v != nil {
		if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
			t.Fatalf("GET %s: decode: %v", path, err)
		}
	}
	return resp
}

func TestListMarkets(t *testing.T) {
	t.Parallel()

	api := newAPI(t)

	var body struct {
		Markets []server.MarketInfo `json:"markets"`
	}
	resp := get(t, api, "/v1/markets", &body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if len(body.Markets) != len(tgju.Markets()) {
		t.Fatalf("got %d markets, want %d", len(body.Markets), len(tgju.Markets()))
	}
	for _, m := range body.Markets {
		if m.Label == "" || m.Source == "" || m.Endpoint == "" {
			t.Errorf("market %q is missing metadata: %+v", m.Name, m)
		}
	}
}

func TestGetMarket(t *testing.T) {
	t.Parallel()

	api := newAPI(t)

	tests := []struct {
		path       string
		wantMarket tgju.Market
	}{
		{"/v1/markets/currency", tgju.Currency},
		{"/v1/markets/gold", tgju.Gold},
		{"/v1/markets/coin", tgju.Coin},
		{"/v1/markets/fx", tgju.Currency},       // alias
		{"/v1/markets/gold-chart", tgju.Gold},   // alias
		{"/v1/markets/CURRENCY", tgju.Currency}, // case insensitive
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()

			var snap tgju.Snapshot
			resp := get(t, api, tc.path, &snap)

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}
			if snap.Market != tc.wantMarket {
				t.Errorf("market = %q, want %q", snap.Market, tc.wantMarket)
			}
			if snap.IsEmpty() {
				t.Error("snapshot is empty")
			}
			if got := resp.Header.Get("Cache-Control"); !strings.HasPrefix(got, "public, max-age=") {
				t.Errorf("Cache-Control = %q, want a public max-age", got)
			}
			if resp.Header.Get("X-Request-Id") == "" {
				t.Error("no X-Request-Id on the response")
			}
		})
	}
}

func TestGetMarketRejectsUnknownNames(t *testing.T) {
	t.Parallel()

	api := newAPI(t)

	var apiErr server.APIError
	resp := get(t, api, "/v1/markets/crypto", &apiErr)

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	if apiErr.Code != server.CodeUnknownMarket {
		t.Errorf("code = %q, want %q", apiErr.Code, server.CodeUnknownMarket)
	}
	if apiErr.RequestID == "" {
		t.Error("the error body carries no request id")
	}
}

func TestGetMarketItems(t *testing.T) {
	t.Parallel()

	api := newAPI(t)

	var body struct {
		Market tgju.Market `json:"market"`
		Count  int         `json:"count"`
		Items  []tgju.Item `json:"items"`
	}
	resp := get(t, api, "/v1/markets/gold/items", &body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if body.Count != len(body.Items) || body.Count == 0 {
		t.Fatalf("count = %d, len(items) = %d", body.Count, len(body.Items))
	}
	for _, item := range body.Items {
		if item.Market != tgju.Gold {
			t.Errorf("item %q has market %q, want gold", item.Key, item.Market)
		}
	}
}

func TestFilters(t *testing.T) {
	t.Parallel()

	api := newAPI(t)

	t.Run("keys", func(t *testing.T) {
		var body struct {
			Items []tgju.Item `json:"items"`
		}
		get(t, api, "/v1/markets/currency/items?keys=price_dollar_rl,price_eur", &body)

		if len(body.Items) != 2 {
			t.Fatalf("got %d items, want 2: %v", len(body.Items), keysOf(body.Items))
		}
		for _, item := range body.Items {
			if item.Key != "price_dollar_rl" && item.Key != "price_eur" {
				t.Errorf("unexpected item %q", item.Key)
			}
		}
	})

	t.Run("unknown keys yield nothing", func(t *testing.T) {
		var body struct {
			Items []tgju.Item `json:"items"`
		}
		get(t, api, "/v1/markets/currency/items?keys=nope", &body)
		if len(body.Items) != 0 {
			t.Errorf("got %d items, want none", len(body.Items))
		}
	})

	t.Run("category", func(t *testing.T) {
		var snap tgju.Snapshot
		get(t, api, "/v1/markets/gold?category="+urlEncode("قیمت نقره"), &snap)

		if len(snap.Categories) != 1 {
			t.Fatalf("got %d categories, want 1", len(snap.Categories))
		}
		if snap.Categories[0].Title != "قیمت نقره" {
			t.Errorf("category = %q", snap.Categories[0].Title)
		}
	})
}

func TestGetMarketItem(t *testing.T) {
	t.Parallel()

	api := newAPI(t)

	var item tgju.Item
	resp := get(t, api, "/v1/markets/gold/items/geram18", &item)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if item.Key != "geram18" || item.Price.Value <= 0 {
		t.Errorf("item = %+v", item)
	}

	var apiErr server.APIError
	resp = get(t, api, "/v1/markets/gold/items/price_eur", &apiErr)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for an instrument on another board", resp.StatusCode)
	}
	if apiErr.Code != server.CodeNotFound {
		t.Errorf("code = %q, want %q", apiErr.Code, server.CodeNotFound)
	}
}

func TestGetItemAcrossMarkets(t *testing.T) {
	t.Parallel()

	api := newAPI(t)

	tests := map[string]tgju.Market{
		"price_dollar_rl": tgju.Currency,
		"geram18":         tgju.Gold,
		"sekee":           tgju.Coin,
	}
	for key, want := range tests {
		var item tgju.Item
		resp := get(t, api, "/v1/items/"+key, &item)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET /v1/items/%s: status = %d", key, resp.StatusCode)
		}
		if item.Market != want {
			t.Errorf("%s market = %q, want %q", key, item.Market, want)
		}
	}

	var apiErr server.APIError
	resp := get(t, api, "/v1/items/no_such_thing", &apiErr)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestSnapshotEndpoint(t *testing.T) {
	t.Parallel()

	api := newAPI(t)

	var body struct {
		FetchedAt time.Time                `json:"fetched_at"`
		Markets   map[string]tgju.Snapshot `json:"markets"`
	}
	resp := get(t, api, "/v1/snapshot", &body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if body.FetchedAt.IsZero() {
		t.Error("fetched_at is zero")
	}
	for _, m := range tgju.Markets() {
		if body.Markets[string(m)].IsEmpty() {
			t.Errorf("market %q is missing or empty", m)
		}
	}
}

// TestLegacyEndpoints pins the wire format of the API this project replaces.
func TestLegacyEndpoints(t *testing.T) {
	t.Parallel()

	api := newAPI(t)

	t.Run("currency is a flat array", func(t *testing.T) {
		var items []map[string]any
		resp := get(t, api, "/api/price/currency", &items)

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		if len(items) == 0 {
			t.Fatal("no items")
		}
		want := []string{"title", "price", "key", "status", "low_price", "high_price"}
		for _, field := range want {
			if _, ok := items[0][field]; !ok {
				t.Errorf("field %q is missing from %v", field, items[0])
			}
		}
		if len(items[0]) != len(want) {
			t.Errorf("item has %d fields, want exactly %d: %v", len(items[0]), len(want), items[0])
		}
		if items[0]["key"] != "price_dollar_rl" {
			t.Errorf("first key = %v, want price_dollar_rl", items[0]["key"])
		}
	})

	for _, path := range []string{"/api/price/gold", "/api/price/coin"} {
		t.Run(path+" is grouped", func(t *testing.T) {
			var categories []struct {
				Title  string           `json:"title"`
				Prices []map[string]any `json:"prices"`
			}
			resp := get(t, api, path, &categories)

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}
			if len(categories) == 0 || categories[0].Title == "" || len(categories[0].Prices) == 0 {
				t.Fatalf("unexpected body: %+v", categories)
			}
		})
	}
}

func TestHealthAndReady(t *testing.T) {
	t.Parallel()

	api := newAPI(t)

	var health struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	resp := get(t, api, "/healthz", &health)
	if resp.StatusCode != http.StatusOK || health.Status != "ok" {
		t.Fatalf("healthz: status = %d, body = %+v", resp.StatusCode, health)
	}
	if health.Version != tgju.Version {
		t.Errorf("version = %q, want %q", health.Version, tgju.Version)
	}

	var ready struct {
		Status string `json:"status"`
		Items  int    `json:"items"`
	}
	resp = get(t, api, "/readyz", &ready)
	if resp.StatusCode != http.StatusOK || ready.Status != "ok" || ready.Items == 0 {
		t.Fatalf("readyz: status = %d, body = %+v", resp.StatusCode, ready)
	}
}

func TestReadyReportsAnUnreachableUpstream(t *testing.T) {
	t.Parallel()

	// A port nothing listens on: the client fails at the transport level.
	client := tgju.New(
		tgju.WithBaseURL("http://127.0.0.1:1"),
		tgju.WithRetry(tgju.RetryPolicy{}),
		tgju.WithTimeout(time.Second),
		tgju.WithCacheTTL(0),
	)
	api := httptest.NewServer(server.New(client, server.WithLogger(slog.New(slog.DiscardHandler))))
	t.Cleanup(api.Close)

	var body struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	}
	resp := get(t, api, "/readyz", &body)

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
	if body.Status != "unavailable" || body.Reason != server.CodeUpstreamDown {
		t.Errorf("body = %+v", body)
	}
}

func TestUpstreamFailuresBecomeBadGateway(t *testing.T) {
	t.Parallel()

	upstream := fixture.ServerFunc(t, func(w http.ResponseWriter, _ *http.Request) bool {
		w.WriteHeader(http.StatusInternalServerError)
		return false
	})
	client := tgju.New(tgju.WithBaseURL(upstream.URL), tgju.WithRetry(tgju.RetryPolicy{}), tgju.WithCacheTTL(0))
	api := httptest.NewServer(server.New(client, server.WithLogger(slog.New(slog.DiscardHandler))))
	t.Cleanup(api.Close)

	var apiErr server.APIError
	resp := get(t, api, "/v1/markets/gold", &apiErr)

	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", resp.StatusCode)
	}
	if apiErr.Code != server.CodeUpstreamDown {
		t.Errorf("code = %q, want %q", apiErr.Code, server.CodeUpstreamDown)
	}
	if strings.Contains(apiErr.Message, upstream.URL) {
		t.Error("the error message leaks the upstream URL")
	}
}

func TestUnparseableUpstreamIsReportedSeparately(t *testing.T) {
	t.Parallel()

	upstream := fixture.ServerFunc(t, func(w http.ResponseWriter, _ *http.Request) bool {
		_, _ = io.WriteString(w, "<html><body>redesigned</body></html>")
		return false
	})
	client := tgju.New(tgju.WithBaseURL(upstream.URL), tgju.WithCacheTTL(0))
	api := httptest.NewServer(server.New(client, server.WithLogger(slog.New(slog.DiscardHandler))))
	t.Cleanup(api.Close)

	var apiErr server.APIError
	resp := get(t, api, "/v1/markets/gold", &apiErr)

	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", resp.StatusCode)
	}
	if apiErr.Code != server.CodeUpstreamChanged {
		t.Errorf("code = %q, want %q", apiErr.Code, server.CodeUpstreamChanged)
	}
}

func TestUnknownPathsAnswerWithTheAPIErrorShape(t *testing.T) {
	t.Parallel()

	api := newAPI(t)

	var apiErr server.APIError
	resp := get(t, api, "/v2/markets", &apiErr)

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	if apiErr.Code != server.CodeNotFound {
		t.Errorf("code = %q, want %q", apiErr.Code, server.CodeNotFound)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want JSON", ct)
	}
}

func TestWriteMethodsAreRejected(t *testing.T) {
	t.Parallel()

	api := newAPI(t)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, api.URL+"/v1/markets/gold", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := api.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", resp.StatusCode)
	}
}

func TestRootRedirectsToDocs(t *testing.T) {
	t.Parallel()

	api := newAPI(t)
	client := *api.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	resp, err := client.Get(api.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); got != "/docs" {
		t.Errorf("Location = %q, want /docs", got)
	}
}

func TestDocsAndOpenAPI(t *testing.T) {
	t.Parallel()

	api := newAPI(t)

	t.Run("docs", func(t *testing.T) {
		resp := get(t, api, "/docs", nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		page := string(body)
		for _, want := range []string{"TGJU API", "/v1/markets/{market}", tgju.Version} {
			if !strings.Contains(page, want) {
				t.Errorf("the documentation page does not mention %q", want)
			}
		}
		// The page must work without an internet connection.
		for _, forbidden := range []string{"cdn.", "https://unpkg", "<script src="} {
			if strings.Contains(page, forbidden) {
				t.Errorf("the documentation page loads something external: %q", forbidden)
			}
		}
	})

	t.Run("openapi", func(t *testing.T) {
		resp := get(t, api, "/openapi.yaml", nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(string(body), "openapi: 3.1") {
			t.Error("the served document is not an OpenAPI 3.1 description")
		}
	})
}

// TestOpenAPICoversEveryRoute keeps the description honest: an endpoint added to
// the router without a matching entry in openapi.yaml fails the build.
func TestOpenAPICoversEveryRoute(t *testing.T) {
	t.Parallel()

	api := newAPI(t)

	resp := get(t, api, "/openapi.yaml", nil)
	spec, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	srv := server.New(nil)
	for _, route := range srv.Routes() {
		if route.Pattern == "/docs" {
			continue // the documentation page is not part of the JSON API
		}
		if !strings.Contains(string(spec), "\n  "+route.Pattern+":") {
			t.Errorf("route %s is not described in openapi.yaml", route.Pattern)
		}
	}
}

func TestMetrics(t *testing.T) {
	t.Parallel()

	api := newAPI(t)

	get(t, api, "/v1/markets/gold", nil)
	get(t, api, "/v1/markets/crypto", nil)

	resp := get(t, api, "/metrics", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	page := string(body)
	for _, want := range []string{
		`tgju_build_info{version="` + tgju.Version + `"} 1`,
		`tgju_http_requests_total{route="/v1/markets/{market}",method="GET",status="200"} 1`,
		`tgju_http_requests_total{route="/v1/markets/{market}",method="GET",status="404"} 1`,
		"tgju_http_request_duration_seconds_bucket",
		"tgju_uptime_seconds",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("metrics do not contain %q", want)
		}
	}
}

func TestMetricsCanBeDisabled(t *testing.T) {
	t.Parallel()

	api := newAPI(t, server.WithMetrics(false))

	resp := get(t, api, "/metrics", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 when metrics are off", resp.StatusCode)
	}
	for _, route := range server.New(nil, server.WithMetrics(false)).Routes() {
		if route.Pattern == "/metrics" {
			t.Error("/metrics is still listed in Routes()")
		}
	}
}

func TestServerCanBeMountedUnderAPrefix(t *testing.T) {
	t.Parallel()

	upstream := fixture.Server(t)
	client := tgju.New(tgju.WithBaseURL(upstream.URL))

	mux := http.NewServeMux()
	mux.Handle("/tgju/", http.StripPrefix("/tgju", server.New(client,
		server.WithLogger(slog.New(slog.DiscardHandler)))))

	host := httptest.NewServer(mux)
	t.Cleanup(host.Close)

	var snap tgju.Snapshot
	resp := get(t, host, "/tgju/v1/markets/coin", &snap)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if snap.Market != tgju.Coin {
		t.Errorf("market = %q, want coin", snap.Market)
	}
}

func TestServerReusesTheClientCache(t *testing.T) {
	t.Parallel()

	var upstreamCalls atomic.Int64
	upstream := fixture.ServerFunc(t, func(_ http.ResponseWriter, _ *http.Request) bool {
		upstreamCalls.Add(1)
		return true
	})
	client := tgju.New(tgju.WithBaseURL(upstream.URL), tgju.WithCacheTTL(time.Hour))
	api := httptest.NewServer(server.New(client, server.WithLogger(slog.New(slog.DiscardHandler))))
	t.Cleanup(api.Close)

	for range 10 {
		get(t, api, "/v1/markets/gold", nil)
	}
	if got := upstreamCalls.Load(); got != 1 {
		t.Errorf("tgju.org was called %d times, want 1", got)
	}
}

func TestRateLimiting(t *testing.T) {
	t.Parallel()

	// One token per second with a burst of two: the third request in a row is
	// rejected.
	api := newAPI(t, server.WithRateLimit(1, 2))

	var lastStatus int
	for range 3 {
		resp := get(t, api, "/healthz", nil)
		lastStatus = resp.StatusCode
	}
	if lastStatus != http.StatusTooManyRequests {
		t.Fatalf("third request status = %d, want 429", lastStatus)
	}

	resp := get(t, api, "/healthz", nil)
	if got := resp.Header.Get("Retry-After"); got == "" {
		t.Error("no Retry-After header on a throttled response")
	} else if _, err := strconv.Atoi(got); err != nil {
		t.Errorf("Retry-After = %q, want a number of seconds", got)
	}
}

func TestCORS(t *testing.T) {
	t.Parallel()

	t.Run("allows any origin by default", func(t *testing.T) {
		api := newAPI(t)

		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, api.URL+"/healthz", nil)
		req.Header.Set("Origin", "https://example.com")

		resp, err := api.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()

		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("Access-Control-Allow-Origin = %q, want *", got)
		}
	})

	t.Run("honours an allow list", func(t *testing.T) {
		api := newAPI(t, server.WithCORS("https://shop.example"))

		for origin, want := range map[string]string{
			"https://shop.example": "https://shop.example",
			"https://evil.example": "",
		} {
			req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, api.URL+"/healthz", nil)
			req.Header.Set("Origin", origin)

			resp, err := api.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()

			if got := resp.Header.Get("Access-Control-Allow-Origin"); got != want {
				t.Errorf("origin %s: Access-Control-Allow-Origin = %q, want %q", origin, got, want)
			}
		}
	})

	t.Run("answers preflight", func(t *testing.T) {
		api := newAPI(t)

		req, _ := http.NewRequestWithContext(t.Context(), http.MethodOptions, api.URL+"/v1/markets", nil)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("Access-Control-Request-Method", "GET")

		resp, err := api.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("status = %d, want 204", resp.StatusCode)
		}
		if got := resp.Header.Get("Access-Control-Allow-Methods"); !strings.Contains(got, "GET") {
			t.Errorf("Access-Control-Allow-Methods = %q", got)
		}
	})
}

func TestRequestIDIsAdoptedWhenSane(t *testing.T) {
	t.Parallel()

	api := newAPI(t)

	tests := map[string]bool{
		"abc-123_XYZ.4":         true,
		"":                      false,
		"has spaces":            false,
		"inject\nlog":           false,
		strings.Repeat("a", 65): false,
	}

	for id, adopted := range tests {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, api.URL+"/healthz", nil)
		if err != nil {
			t.Fatal(err)
		}
		// A header value the transport itself would reject cannot be tested
		// through a real request, so set it directly.
		req.Header["X-Request-Id"] = []string{id}

		resp, err := api.Client().Do(req)
		if err != nil {
			if !adopted {
				continue // the transport refused to send it, which is also fine
			}
			t.Fatalf("request id %q: %v", id, err)
		}
		_ = resp.Body.Close()

		got := resp.Header.Get("X-Request-Id")
		switch {
		case adopted && got != id:
			t.Errorf("request id %q was not adopted, got %q", id, got)
		case !adopted && got == id:
			t.Errorf("request id %q should have been replaced", id)
		case got == "":
			t.Errorf("request id %q produced no id at all", id)
		}
	}
}

func keysOf(items []tgju.Item) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.Key
	}
	return out
}

func urlEncode(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
			continue
		}
		b.WriteString("%")
		const hex = "0123456789ABCDEF"
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&0x0f])
	}
	return b.String()
}
