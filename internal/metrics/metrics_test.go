package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
)

func TestMetrics_CacheLookups(t *testing.T) {
	tests := []struct {
		name     string
		hits     int
		misses   int
		wantHit  float64
		wantMiss float64
	}{
		{name: "no activity", hits: 0, misses: 0, wantHit: 0, wantMiss: 0},
		{name: "hits only", hits: 3, misses: 0, wantHit: 3, wantMiss: 0},
		{name: "misses only", hits: 0, misses: 2, wantHit: 0, wantMiss: 2},
		{name: "hits and misses", hits: 2, misses: 5, wantHit: 2, wantMiss: 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New()

			for i := 0; i < tt.hits; i++ {
				m.IncCacheHit()
			}
			for i := 0; i < tt.misses; i++ {
				m.IncCacheMiss()
			}

			gotHit := testutil.ToFloat64(m.cacheLookups.WithLabelValues(CacheHit))
			gotMiss := testutil.ToFloat64(m.cacheLookups.WithLabelValues(CacheMiss))

			if gotHit != tt.wantHit {
				t.Errorf("cache hit count = %v, want %v", gotHit, tt.wantHit)
			}
			if gotMiss != tt.wantMiss {
				t.Errorf("cache miss count = %v, want %v", gotMiss, tt.wantMiss)
			}
		})
	}
}

func TestMetrics_PluginFailures(t *testing.T) {
	tests := []struct {
		name        string
		incPlugin   []string
		checkPlugin string
		want        float64
	}{
		{name: "never incremented", incPlugin: nil, checkPlugin: "file", want: 0},
		{name: "single failure", incPlugin: []string{"file"}, checkPlugin: "file", want: 1},
		{name: "two failures same plugin", incPlugin: []string{"file", "file"}, checkPlugin: "file", want: 2},
		{name: "other plugin unaffected", incPlugin: []string{"file"}, checkPlugin: "memory", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New()

			for _, plugin := range tt.incPlugin {
				m.IncPluginFailure(plugin)
			}

			got := testutil.ToFloat64(m.pluginFailures.WithLabelValues(tt.checkPlugin))
			if got != tt.want {
				t.Errorf("plugin failure count for %q = %v, want %v", tt.checkPlugin, got, tt.want)
			}
		})
	}
}

// histogramSampleCount returns the number of observations recorded by a
// histogram Observer. testutil.ToFloat64 only supports single-value metrics
// (counters/gauges), so histograms need to be read via their wire encoding.
func histogramSampleCount(t *testing.T, o prometheus.Observer) uint64 {
	t.Helper()

	hist, ok := o.(prometheus.Histogram)
	if !ok {
		t.Fatalf("observer %T does not implement prometheus.Histogram", o)
	}

	var pb dto.Metric
	if err := hist.Write(&pb); err != nil {
		t.Fatalf("failed to write histogram metric: %v", err)
	}

	return pb.GetHistogram().GetSampleCount()
}

func TestMetrics_QueryLatency(t *testing.T) {
	strategies := []string{StrategyCache, StrategyPlugin, StrategyUpstream, StrategyFailed}

	tests := []struct {
		name     string
		strategy string
	}{
		{name: "cache strategy", strategy: StrategyCache},
		{name: "plugin strategy", strategy: StrategyPlugin},
		{name: "upstream strategy", strategy: StrategyUpstream},
		{name: "failed strategy", strategy: StrategyFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New()

			m.ObserveQueryLatency(tt.strategy, 10*time.Millisecond)

			for _, s := range strategies {
				count := histogramSampleCount(t, m.queryLatency.WithLabelValues(s))
				if s == tt.strategy {
					if count != 1 {
						t.Errorf("latency observation count for strategy %q = %d, want 1", s, count)
					}
				} else {
					if count != 0 {
						t.Errorf("latency observation count for strategy %q = %d, want 0", s, count)
					}
				}
			}
		})
	}
}
