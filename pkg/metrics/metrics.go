// Package metrics exposes Prometheus metrics for the HTTP server, outgoing
// HTTP calls and PostgreSQL, plus a helper to serve them on a separate port.
//
// All collectors live in a package-level registry so middlewares can be
// created many times (e.g. in tests) without duplicate-registration panics.
package metrics

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	namespace = "birga"

	labelMethod = "method"
	labelStatus = "status"

	readHeaderTimeout = 5 * time.Second
	shutdownTimeout   = 5 * time.Second
)

// Buckets tuned for an API whose p99 should be well under a second.
var defaultBuckets = []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}

var registry = newRegistry()

func newRegistry() *prometheus.Registry {
	r := prometheus.NewRegistry()
	r.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	return r
}

// Register adds an application-specific collector (e.g. a business counter).
func Register(c prometheus.Collector) error {
	return registry.Register(c)
}

// MustRegister is Register that panics on error.
func MustRegister(c ...prometheus.Collector) {
	registry.MustRegister(c...)
}

// Handler serves the registry in Prometheus text format.
func Handler() http.Handler {
	return promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
}

// Expose serves /metrics on addr (e.g. ":9090") until ctx is cancelled.
func Expose(ctx context.Context, addr string) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", Handler())

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	go func() { //nolint:gosec // shutdown must use a fresh context: ctx is already cancelled here
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		_ = srv.Shutdown(shutdownCtx)
	}()

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}
