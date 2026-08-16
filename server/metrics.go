package server

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	tgju "github.com/amiranmanesh/tgju-api-go"
)

// metrics is a minimal Prometheus exposition, hand written so the binary keeps
// its single dependency.
//
// It counts what an operator actually pages on — request volume, error rate and
// latency per route — and nothing else. Anything richer is a job for a real
// client library, which this package is careful not to force on its users.
type metrics struct {
	enabled bool
	started time.Time

	mu      sync.Mutex
	series  map[seriesKey]*seriesValue
	buckets []float64
}

type seriesKey struct {
	route  string
	method string
	status int
}

type seriesValue struct {
	count       uint64
	sumSeconds  float64
	bucketCount []uint64 // cumulative counts, aligned with metrics.buckets
}

// latencyBuckets are the histogram boundaries in seconds. They straddle the two
// regimes this service has: a cache hit, answered in microseconds, and a cache
// miss, which waits on tgju.org.
var latencyBuckets = []float64{0.005, 0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

func newMetrics(enabled bool, now func() time.Time) *metrics {
	return &metrics{
		enabled: enabled,
		started: now(),
		series:  map[seriesKey]*seriesValue{},
		buckets: latencyBuckets,
	}
}

func (m *metrics) observe(route, method string, status int, elapsed time.Duration) {
	if m == nil || !m.enabled {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	key := seriesKey{route: route, method: method, status: status}
	value, ok := m.series[key]
	if !ok {
		value = &seriesValue{bucketCount: make([]uint64, len(m.buckets))}
		m.series[key] = value
	}

	seconds := elapsed.Seconds()
	value.count++
	value.sumSeconds += seconds
	for i, upper := range m.buckets {
		if seconds <= upper {
			value.bucketCount[i]++
		}
	}
}

// handler writes the metrics in the Prometheus text exposition format.
func (m *metrics) handler(now func() time.Time) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		keys := make([]seriesKey, 0, len(m.series))
		for key := range m.series {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].route != keys[j].route {
				return keys[i].route < keys[j].route
			}
			if keys[i].method != keys[j].method {
				return keys[i].method < keys[j].method
			}
			return keys[i].status < keys[j].status
		})

		var b strings.Builder
		fmt.Fprintf(&b, "# HELP tgju_build_info The version of the running service.\n")
		fmt.Fprintf(&b, "# TYPE tgju_build_info gauge\n")
		fmt.Fprintf(&b, "tgju_build_info{version=%q} 1\n", tgju.Version)

		fmt.Fprintf(&b, "# HELP tgju_uptime_seconds Seconds since the server started.\n")
		fmt.Fprintf(&b, "# TYPE tgju_uptime_seconds gauge\n")
		fmt.Fprintf(&b, "tgju_uptime_seconds %.3f\n", now().Sub(m.started).Seconds())

		fmt.Fprintf(&b, "# HELP tgju_http_requests_total Requests handled, by route, method and status.\n")
		fmt.Fprintf(&b, "# TYPE tgju_http_requests_total counter\n")
		for _, key := range keys {
			fmt.Fprintf(&b, "tgju_http_requests_total{route=%q,method=%q,status=%q} %d\n",
				key.route, key.method, strconv.Itoa(key.status), m.series[key].count)
		}

		fmt.Fprintf(&b, "# HELP tgju_http_request_duration_seconds Request latency.\n")
		fmt.Fprintf(&b, "# TYPE tgju_http_request_duration_seconds histogram\n")
		for _, key := range keys {
			value := m.series[key]
			labels := fmt.Sprintf("route=%q,method=%q,status=%q", key.route, key.method, strconv.Itoa(key.status))
			for i, upper := range m.buckets {
				fmt.Fprintf(&b, "tgju_http_request_duration_seconds_bucket{%s,le=\"%g\"} %d\n",
					labels, upper, value.bucketCount[i])
			}
			fmt.Fprintf(&b, "tgju_http_request_duration_seconds_bucket{%s,le=\"+Inf\"} %d\n", labels, value.count)
			fmt.Fprintf(&b, "tgju_http_request_duration_seconds_sum{%s} %f\n", labels, value.sumSeconds)
			fmt.Fprintf(&b, "tgju_http_request_duration_seconds_count{%s} %d\n", labels, value.count)
		}
		m.mu.Unlock()

		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(b.String()))
	}
}
