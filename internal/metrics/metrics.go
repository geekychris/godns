// Package metrics exposes Prometheus instrumentation for the DNS
// resolution path: per-strategy query latency, cache hit/miss counts, and
// upstream plugin failure counts.
package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Resolution strategy labels for godns_query_duration_seconds.
const (
	StrategyCache    = "cache"
	StrategyPlugin   = "plugin"
	StrategyUpstream = "upstream"
	StrategyFailed   = "failed"
)

// Cache lookup result labels for godns_cache_lookups_total.
const (
	CacheHit  = "hit"
	CacheMiss = "miss"
)

// Metrics owns a private Prometheus registry and the collectors recording
// DNS resolution behavior. Each DNSServer instance should hold its own
// Metrics so that running multiple servers in one process (as the test
// suite does) never collides on duplicate registration.
type Metrics struct {
	registry *prometheus.Registry

	queryLatency   *prometheus.HistogramVec
	cacheLookups   *prometheus.CounterVec
	pluginFailures *prometheus.CounterVec
}

// New creates a Metrics instance with a fresh, private registry.
func New() *Metrics {
	reg := prometheus.NewRegistry()
	factory := promauto.With(reg)

	return &Metrics{
		registry: reg,
		queryLatency: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "godns_query_duration_seconds",
			Help:    "Latency of DNS query resolution, labeled by resolution strategy.",
			Buckets: prometheus.DefBuckets,
		}, []string{"strategy"}),
		cacheLookups: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "godns_cache_lookups_total",
			Help: "Count of DNS cache lookups, labeled by result (hit or miss).",
		}, []string{"result"}),
		pluginFailures: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "godns_plugin_failures_total",
			Help: "Count of upstream plugin resolution failures, labeled by plugin name.",
		}, []string{"plugin"}),
	}
}

// Registry returns the private Prometheus registry backing these metrics,
// for mounting on a /metrics HTTP endpoint.
func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}

// ObserveQueryLatency records how long a DNS query took to resolve via the
// given strategy (one of the Strategy* constants).
func (m *Metrics) ObserveQueryLatency(strategy string, d time.Duration) {
	m.queryLatency.WithLabelValues(strategy).Observe(d.Seconds())
}

// IncCacheHit records a DNS cache hit.
func (m *Metrics) IncCacheHit() {
	m.cacheLookups.WithLabelValues(CacheHit).Inc()
}

// IncCacheMiss records a DNS cache miss.
func (m *Metrics) IncCacheMiss() {
	m.cacheLookups.WithLabelValues(CacheMiss).Inc()
}

// IncPluginFailure records a resolution failure from the named plugin.
func (m *Metrics) IncPluginFailure(plugin string) {
	m.pluginFailures.WithLabelValues(plugin).Inc()
}
