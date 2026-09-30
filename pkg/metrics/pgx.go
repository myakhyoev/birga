package metrics

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

var dbQueryDuration = register(prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Namespace: namespace,
	Subsystem: "db",
	Name:      "query_duration_seconds",
	Help:      "Duration of PostgreSQL queries, by SQL operation and outcome.",
	Buckets:   defaultBuckets,
}, []string{"operation", labelStatus}))

type queryStartKey struct{}

type queryStart struct {
	at        time.Time
	operation string
}

// PgxTracer implements pgx.QueryTracer. Set it on pgx.ConnConfig.Tracer.
type PgxTracer struct{}

// NewPgxTracer returns a tracer that records query durations.
func NewPgxTracer() *PgxTracer {
	return &PgxTracer{}
}

// TraceQueryStart implements pgx.QueryTracer.
func (*PgxTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, queryStartKey{}, queryStart{at: time.Now(), operation: sqlOperation(data.SQL)})
}

// TraceQueryEnd implements pgx.QueryTracer.
func (*PgxTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	start, ok := ctx.Value(queryStartKey{}).(queryStart)
	if !ok {
		return
	}

	status := "ok"
	if data.Err != nil && !errors.Is(data.Err, pgx.ErrNoRows) {
		status = "error"
	}

	dbQueryDuration.WithLabelValues(start.operation, status).Observe(time.Since(start.at).Seconds())
}

// sqlOperation returns the leading SQL keyword ("select", "insert", ...) so the
// label stays low-cardinality. Queries starting with a CTE report "with".
func sqlOperation(sql string) string {
	fields := strings.Fields(sql)
	if len(fields) == 0 {
		return "unknown"
	}

	return strings.ToLower(strings.TrimSuffix(fields[0], ";"))
}

// PgxPoolCollector exports pgxpool connection statistics.
type PgxPoolCollector struct {
	pool *pgxpool.Pool

	acquired, idle, total, maxConns *prometheus.Desc
	acquireCount, emptyAcquireCount *prometheus.Desc
	acquireDuration                 *prometheus.Desc
}

// NewPgxPoolCollector returns a collector for pool; register it with MustRegister.
func NewPgxPoolCollector(pool *pgxpool.Pool) *PgxPoolCollector {
	desc := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(namespace, "db_pool", name), help, nil, nil)
	}

	return &PgxPoolCollector{
		pool:              pool,
		acquired:          desc("acquired_connections", "Connections currently in use."),
		idle:              desc("idle_connections", "Idle connections in the pool."),
		total:             desc("total_connections", "Total connections in the pool."),
		maxConns:          desc("max_connections", "Maximum pool size."),
		acquireCount:      desc("acquire_total", "Successful connection acquisitions."),
		emptyAcquireCount: desc("empty_acquire_total", "Acquisitions that had to wait because the pool was empty."),
		acquireDuration:   desc("acquire_duration_seconds_total", "Total time spent waiting to acquire a connection."),
	}
}

// Describe implements prometheus.Collector.
func (c *PgxPoolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.acquired
	ch <- c.idle
	ch <- c.total
	ch <- c.maxConns
	ch <- c.acquireCount
	ch <- c.emptyAcquireCount
	ch <- c.acquireDuration
}

// Collect implements prometheus.Collector.
func (c *PgxPoolCollector) Collect(ch chan<- prometheus.Metric) {
	s := c.pool.Stat()

	ch <- prometheus.MustNewConstMetric(c.acquired, prometheus.GaugeValue, float64(s.AcquiredConns()))
	ch <- prometheus.MustNewConstMetric(c.idle, prometheus.GaugeValue, float64(s.IdleConns()))
	ch <- prometheus.MustNewConstMetric(c.total, prometheus.GaugeValue, float64(s.TotalConns()))
	ch <- prometheus.MustNewConstMetric(c.maxConns, prometheus.GaugeValue, float64(s.MaxConns()))
	ch <- prometheus.MustNewConstMetric(c.acquireCount, prometheus.CounterValue, float64(s.AcquireCount()))
	ch <- prometheus.MustNewConstMetric(c.emptyAcquireCount, prometheus.CounterValue, float64(s.EmptyAcquireCount()))
	ch <- prometheus.MustNewConstMetric(c.acquireDuration, prometheus.CounterValue, s.AcquireDuration().Seconds())
}
