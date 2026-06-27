// Package metrics provides Prometheus instrumentation for the service. Metrics
// are exposed at /metrics on the debug server and scraped by Prometheus.
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var registry = prometheus.NewRegistry()

var (
	httpRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "tetra_http_requests_total",
		Help: "Total number of HTTP requests processed.",
	}, []string{"method", "route", "code"})

	httpDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "tetra_http_request_duration_seconds",
		Help:    "HTTP request latency in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "route"})

	panicsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "tetra_panics_total",
		Help: "Total number of recovered panics.",
	})

	queueDepth = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "tetra_queue_depth",
		Help: "Number of jobs in the queue by status.",
	}, []string{"status"})

	jobsInflight = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "tetra_jobs_inflight",
		Help: "Number of jobs currently being processed.",
	})

	jobDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "tetra_job_duration_seconds",
		Help:    "Job processing latency in seconds by outcome.",
		Buckets: prometheus.DefBuckets,
	}, []string{"outcome"})
)

func init() {
	registry.MustRegister(
		httpRequests,
		httpDuration,
		panicsTotal,
		queueDepth,
		jobsInflight,
		jobDuration,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
}

// Handler returns the HTTP handler that exposes the metrics for scraping.
func Handler() http.Handler {
	return promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
}

// ObserveHTTP records a completed HTTP request.
func ObserveHTTP(method string, route string, code int, d time.Duration) {
	httpRequests.WithLabelValues(method, route, strconv.Itoa(code)).Inc()
	httpDuration.WithLabelValues(method, route).Observe(d.Seconds())
}

// AddPanic increments the recovered-panic counter.
func AddPanic() {
	panicsTotal.Inc()
}

// SetQueueDepth records the current queue depth for a status.
func SetQueueDepth(status string, n float64) {
	queueDepth.WithLabelValues(status).Set(n)
}

// IncInflight increments the in-flight job gauge.
func IncInflight() {
	jobsInflight.Inc()
}

// DecInflight decrements the in-flight job gauge.
func DecInflight() {
	jobsInflight.Dec()
}

// ObserveJob records a finished job's processing time and outcome.
func ObserveJob(outcome string, d time.Duration) {
	jobDuration.WithLabelValues(outcome).Observe(d.Seconds())
}

// RegisterPoolStats registers a collector that reports pgx pool statistics on
// each scrape.
func RegisterPoolStats(pool *pgxpool.Pool) {
	registry.MustRegister(newPoolCollector(pool))
}

type poolCollector struct {
	pool     *pgxpool.Pool
	acquired *prometheus.Desc
	idle     *prometheus.Desc
	total    *prometheus.Desc
	max      *prometheus.Desc
}

func newPoolCollector(pool *pgxpool.Pool) *poolCollector {
	return &poolCollector{
		pool:     pool,
		acquired: prometheus.NewDesc("tetra_db_pool_acquired_conns", "Currently acquired connections.", nil, nil),
		idle:     prometheus.NewDesc("tetra_db_pool_idle_conns", "Currently idle connections.", nil, nil),
		total:    prometheus.NewDesc("tetra_db_pool_total_conns", "Total connections in the pool.", nil, nil),
		max:      prometheus.NewDesc("tetra_db_pool_max_conns", "Maximum number of connections.", nil, nil),
	}
}

func (c *poolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.acquired
	ch <- c.idle
	ch <- c.total
	ch <- c.max
}

func (c *poolCollector) Collect(ch chan<- prometheus.Metric) {
	s := c.pool.Stat()
	ch <- prometheus.MustNewConstMetric(c.acquired, prometheus.GaugeValue, float64(s.AcquiredConns()))
	ch <- prometheus.MustNewConstMetric(c.idle, prometheus.GaugeValue, float64(s.IdleConns()))
	ch <- prometheus.MustNewConstMetric(c.total, prometheus.GaugeValue, float64(s.TotalConns()))
	ch <- prometheus.MustNewConstMetric(c.max, prometheus.GaugeValue, float64(s.MaxConns()))
}
