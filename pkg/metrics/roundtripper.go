package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var httpClientDuration = register(prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Namespace: namespace,
	Subsystem: "http_client",
	Name:      "request_duration_seconds",
	Help:      "Duration of outgoing HTTP requests, by target service, method and status (\"error\" on transport failure).",
	Buckets:   defaultBuckets,
}, []string{"service", labelMethod, labelStatus}))

type roundTripper struct {
	next    http.RoundTripper
	service string
}

// RoundTripper wraps next (http.DefaultTransport when nil) and records the
// duration of every outgoing request under the given service label.
func RoundTripper(service string, next http.RoundTripper) http.RoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}

	return &roundTripper{next: next, service: service}
}

func (rt *roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()

	resp, err := rt.next.RoundTrip(req)

	status := "error"
	if err == nil {
		status = strconv.Itoa(resp.StatusCode)
	}

	httpClientDuration.WithLabelValues(rt.service, req.Method, status).Observe(time.Since(start).Seconds())

	return resp, err
}
