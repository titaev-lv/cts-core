package rest

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/titaev-lv/cts-core/internal/api/ws"
	"github.com/titaev-lv/cts-core/internal/state"
)

// MetricsHandlerOptions configures Prometheus handler data sources.
type MetricsHandlerOptions struct {
	WSHandler    *ws.Handler
	StateManager *state.Manager
}

func normalizeMetricsPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "/metrics"
	}
	if strings.HasPrefix(trimmed, "/") {
		return trimmed
	}
	return "/" + trimmed
}

func newMetricsHandler(opts MetricsHandlerOptions) gin.HandlerFunc {
	registry := prometheus.NewRegistry()
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	registry.MustRegister(prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{Namespace: "cts_core", Name: "ws_active_connections", Help: "Current active websocket connections"},
		func() float64 {
			if opts.WSHandler == nil {
				return 0
			}
			return float64(opts.WSHandler.GetStats().ActiveConnections)
		},
	))

	registry.MustRegister(prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{Namespace: "cts_core", Name: "ws_total_connections", Help: "Total websocket connections accepted since start"},
		func() float64 {
			if opts.WSHandler == nil {
				return 0
			}
			return float64(opts.WSHandler.GetStats().TotalConnections)
		},
	))

	registry.MustRegister(prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{Namespace: "cts_core", Name: "runtime_scheduler_cycle_count", Help: "Last known scheduler cycle count from runtime state"},
		func() float64 {
			if opts.StateManager == nil {
				return 0
			}
			return float64(opts.StateManager.GetState().Runtime.SchedulerCycleCount)
		},
	))

	registry.MustRegister(prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{Namespace: "cts_core", Name: "runtime_scheduler_last_candidate_count", Help: "Last known candidate count selected by scheduler"},
		func() float64 {
			if opts.StateManager == nil {
				return 0
			}
			return float64(opts.StateManager.GetState().Runtime.SchedulerLastCandidateCount)
		},
	))

	registry.MustRegister(prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{Namespace: "cts_core", Name: "runtime_ws_timeout_count", Help: "Websocket timeout count stored in runtime state"},
		func() float64 {
			if opts.StateManager == nil {
				return 0
			}
			return float64(opts.StateManager.GetState().Runtime.WSTimeoutCount)
		},
	))

	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	return gin.WrapH(handler)
}
