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

var defaultSchedulerAssignResults = []string{
	"success",
	"no_candidates",
	"dedup",
	"failed_retry_exhausted",
	"failed_non_retryable",
}

var defaultSchedulerResourceRejectionReasons = []string{
	"missing_trader_db_id",
	"resource_lookup_error",
	"missing_resource",
	"hard_limit",
}

var defaultWSDisconnectReasons = []string{
	"close_4009_sequence_gap",
	"timeout",
	"read_error",
	"client_close",
	"server_shutdown",
	"protocol_error",
}

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
		prometheus.GaugeOpts{Namespace: "cts_core", Name: "ws_last_ping_unix", Help: "Unix timestamp of last WS ping sent"},
		func() float64 {
			if opts.WSHandler == nil {
				return 0
			}
			return float64(opts.WSHandler.GetStats().Ping.LastPingUnix)
		},
	))

	registry.MustRegister(prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{Namespace: "cts_core", Name: "ws_last_pong_unix", Help: "Unix timestamp of last WS pong received"},
		func() float64 {
			if opts.WSHandler == nil {
				return 0
			}
			return float64(opts.WSHandler.GetStats().Ping.LastPongUnix)
		},
	))

	registry.MustRegister(prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{Namespace: "cts_core", Name: "ws_last_ping_rtt_ms", Help: "Last observed WS ping round-trip time in milliseconds"},
		func() float64 {
			if opts.WSHandler == nil {
				return 0
			}
			return float64(opts.WSHandler.GetStats().Ping.LastRTTMs)
		},
	))

	registry.MustRegister(prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{Namespace: "cts_core", Name: "ws_disconnect_total", Help: "Total websocket disconnects observed by server"},
		func() float64 {
			if opts.StateManager != nil {
				return float64(opts.StateManager.GetState().Runtime.WSDisconnectTotal)
			}
			if opts.WSHandler == nil {
				return 0
			}
			return float64(opts.WSHandler.GetStats().Disconnect.Total)
		},
	))

	registry.MustRegister(prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{Namespace: "cts_core", Name: "ws_disconnect_close_4009_total", Help: "Websocket disconnects caused by close code 4009"},
		func() float64 {
			if opts.StateManager != nil {
				return float64(opts.StateManager.GetState().Runtime.WSDisconnectClose4009)
			}
			if opts.WSHandler == nil {
				return 0
			}
			return float64(opts.WSHandler.GetStats().Disconnect.Close4009)
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

	assignAttempts := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Namespace: "cts_core", Name: "scheduler_assign_attempts_total", Help: "Scheduler assignment attempts by result"},
		[]string{"result"},
	)
	registry.MustRegister(assignAttempts)

	resourceRejections := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Namespace: "cts_core", Name: "scheduler_resource_rejections_total", Help: "Scheduler resource rejections by reason"},
		[]string{"reason"},
	)
	registry.MustRegister(resourceRejections)

	disconnectReasons := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Namespace: "cts_core", Name: "ws_disconnect_reason_total", Help: "Websocket disconnects by reason"},
		[]string{"reason"},
	)
	registry.MustRegister(disconnectReasons)

	scoreDistribution := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Namespace: "cts_core", Name: "scheduler_score_distribution", Help: "Scheduler score distribution snapshot"},
		[]string{"quantile"},
	)
	registry.MustRegister(scoreDistribution)

	registry.MustRegister(prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{Namespace: "cts_core", Name: "scheduler_assign_latency_ms", Help: "Last scheduler assignment latency in milliseconds"},
		func() float64 {
			if opts.StateManager == nil {
				return 0
			}
			return opts.StateManager.GetState().Runtime.SchedulerAssignLatencyMs
		},
	))

	registry.MustRegister(prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{Namespace: "cts_core", Name: "scheduler_candidate_pool_size", Help: "Last candidate pool size from scheduler cycle"},
		func() float64 {
			if opts.StateManager == nil {
				return 0
			}
			return float64(opts.StateManager.GetState().Runtime.SchedulerLastCandidateCount)
		},
	))

	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	return func(c *gin.Context) {
		assignAttempts.Reset()
		for _, result := range defaultSchedulerAssignResults {
			assignAttempts.WithLabelValues(result).Set(0)
		}

		resourceRejections.Reset()
		for _, reason := range defaultSchedulerResourceRejectionReasons {
			resourceRejections.WithLabelValues(reason).Set(0)
		}

		disconnectReasons.Reset()
		for _, reason := range defaultWSDisconnectReasons {
			disconnectReasons.WithLabelValues(reason).Set(0)
		}

		if opts.StateManager != nil {
			runtimeState := opts.StateManager.GetState().Runtime
			for result, count := range runtimeState.SchedulerAssignAttempts {
				assignAttempts.WithLabelValues(result).Set(float64(count))
			}

			for reason, count := range runtimeState.SchedulerResourceRejections {
				resourceRejections.WithLabelValues(reason).Set(float64(count))
			}

			scoreDistribution.WithLabelValues("p50").Set(runtimeState.SchedulerScoreP50)
			scoreDistribution.WithLabelValues("p95").Set(runtimeState.SchedulerScoreP95)
		}

		if opts.StateManager != nil {
			for reason, count := range opts.StateManager.GetState().Runtime.WSDisconnectByReason {
				disconnectReasons.WithLabelValues(reason).Set(float64(count))
			}
		} else if opts.WSHandler != nil {
			disconnect := opts.WSHandler.GetStats().Disconnect
			for reason, count := range disconnect.ByReason {
				disconnectReasons.WithLabelValues(reason).Set(float64(count))
			}
		}
		handler.ServeHTTP(c.Writer, c.Request)
	}
}
