package metrics

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

const unmatchedRoute = "unmatched"

var (
	httpRequestDuration = register(prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: "http_server",
		Name:      "request_duration_seconds",
		Help:      "Duration of HTTP requests served, by route template, method, status and application error code.",
		Buckets:   defaultBuckets,
	}, []string{labelMethod, "route", labelStatus, "code"}))

	httpRequestsInFlight = register(prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: "http_server",
		Name:      "requests_in_flight",
		Help:      "Number of HTTP requests currently being served.",
	}, []string{labelMethod}))
)

// CodeResolver returns the application-level result code of a request
// (e.g. "0", "-10"). Return "" when the application did not set one.
type CodeResolver func(c *gin.Context) string

// Gin records request duration and in-flight requests. Routes are labelled by
// their template (/v1/activities/:id), never the raw path, to keep label
// cardinality bounded. Routes listed in ignore are not recorded.
func Gin(resolveCode CodeResolver, ignore ...string) gin.HandlerFunc {
	skip := make(map[string]struct{}, len(ignore))
	for _, p := range ignore {
		skip[p] = struct{}{}
	}

	return func(c *gin.Context) {
		route := c.FullPath()
		if _, ok := skip[route]; ok {
			c.Next()

			return
		}

		method := c.Request.Method
		inFlight := httpRequestsInFlight.WithLabelValues(method)
		inFlight.Inc()

		start := time.Now()

		defer func() {
			inFlight.Dec()

			if route == "" {
				route = unmatchedRoute
			}

			code := ""
			if resolveCode != nil {
				code = resolveCode(c)
			}

			httpRequestDuration.
				WithLabelValues(method, route, strconv.Itoa(c.Writer.Status()), code).
				Observe(time.Since(start).Seconds())
		}()

		c.Next()
	}
}

func register[T prometheus.Collector](c T) T {
	registry.MustRegister(c)

	return c
}
