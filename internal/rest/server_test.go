package rest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yourusername/godns/internal/config"
	"github.com/yourusername/godns/internal/metrics"
)

// TestMetricsEndpoint verifies that /metrics is mounted on the REST router
// and serves the Prometheus text-exposition format for the metrics recorded
// on the *metrics.Metrics instance the server was built with.
func TestMetricsEndpoint(t *testing.T) {
	cfg := &config.Config{REST: config.RESTConfig{Enabled: true, Address: "127.0.0.1", Port: 0}}
	logger := logrus.New()

	m := metrics.New()
	m.IncCacheHit()
	m.ObserveQueryLatency(metrics.StrategyCache, 10*time.Millisecond)

	server := NewServer(cfg, logger, nil, m)
	restServer, ok := server.(*RESTServer)
	require.True(t, ok, "NewServer should return a *RESTServer")

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	restServer.router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "text/plain")

	body := rec.Body.String()
	assert.True(t, strings.Contains(body, "godns_cache_lookups_total"), "body should contain cache lookups metric")
	assert.True(t, strings.Contains(body, "godns_query_duration_seconds"), "body should contain query latency metric")
}
