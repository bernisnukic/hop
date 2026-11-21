package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics are the series the SLOs are built on. The route label is the
// ServeMux pattern ("GET /{code}"), never the raw path, so its cardinality
// is fixed by the code, not by traffic.
type Metrics struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	created  prometheus.Counter
}

// NewMetrics registers hop's metrics on reg.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "hop_http_requests_total",
			Help: "HTTP requests served, by route pattern and status code.",
		}, []string{"route", "code"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "hop_http_request_duration_seconds",
			Help: "Time to serve an HTTP request, by route pattern.",
			// 0.1 and 0.25 are the latency SLO thresholds; keep them as bucket bounds.
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5},
		}, []string{"route"}),
		created: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "hop_links_created_total",
			Help: "Short links created.",
		}),
	}
	reg.MustRegister(m.requests, m.duration, m.created)
	return m
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

// instrument records every request after the mux has matched it.
func (m *Metrics) instrument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(sw, r)
		route := r.Pattern // set by ServeMux on this request
		if route == "" {
			route = "unmatched"
		}
		m.requests.WithLabelValues(route, strconv.Itoa(sw.code)).Inc()
		m.duration.WithLabelValues(route).Observe(time.Since(start).Seconds())
	})
}
