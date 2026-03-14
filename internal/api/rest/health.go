package rest

import (
	"context"
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/titaev-lv/cts-core/internal/api/ws"
	"github.com/titaev-lv/cts-core/internal/db"
	"github.com/titaev-lv/cts-core/internal/hsm"
	"github.com/titaev-lv/cts-core/internal/state"
)

// HealthHandler handles health check requests
type HealthHandler struct {
	dbClient       dbPinger
	hsmTrading     *hsm.Client
	hsmTwoFA       *hsm.Client
	stateManager   *state.Manager
	wsHandler      *ws.Handler
	startedAt      time.Time
	serviceName    string
	serviceVersion string
}

type dbPinger interface {
	Ping() error
}

type HealthHandlerOptions struct {
	HSMTrading     *hsm.Client
	HSMTwoFA       *hsm.Client
	StateManager   *state.Manager
	WSHandler      *ws.Handler
	StartedAt      time.Time
	ServiceName    string
	ServiceVersion string
}

// NewHealthHandler creates a new health check handler.
func NewHealthHandler(dbClient *db.MySQLClient, opts HealthHandlerOptions) *HealthHandler {
	startedAt := opts.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}

	return &HealthHandler{
		dbClient:       dbClient,
		hsmTrading:     opts.HSMTrading,
		hsmTwoFA:       opts.HSMTwoFA,
		stateManager:   opts.StateManager,
		wsHandler:      opts.WSHandler,
		startedAt:      startedAt,
		serviceName:    opts.ServiceName,
		serviceVersion: opts.ServiceVersion,
	}
}

// Health returns the health status of the service
// GET /health
func (h *HealthHandler) Health(c *gin.Context) {
	now := time.Now().UTC()
	uptimeSec := int64(now.Sub(h.startedAt).Seconds())

	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	dbComponent := h.checkDatabase()
	hsmTradingComponent := h.checkHSM(ctx, h.hsmTrading)
	hsmTwoFAComponent := h.checkHSM(ctx, h.hsmTwoFA)

	wsStats := ws.Stats{}
	if h.wsHandler != nil {
		wsStats = h.wsHandler.GetStats()
	}

	if h.stateManager != nil {
		h.stateManager.SetRuntimeWS(wsStats.ActiveConnections, wsStats.LastConnectUnix)
	}

	stateSnapshot := gin.H{}
	runtimeSnapshot := state.RuntimeState{}
	if h.stateManager != nil {
		snapshot := h.stateManager.GetState()
		runtimeSnapshot = snapshot.Runtime
		stateSnapshot = gin.H{
			"version":         snapshot.Version,
			"updated_at":      snapshot.UpdatedAt,
			"updated_at_unix": snapshot.UpdatedAt.Unix(),
			"server":          snapshot.Server,
			"runtime":         snapshot.Runtime,
		}
	}

	var runtimeMem runtime.MemStats
	runtime.ReadMemStats(&runtimeMem)

	traderStatus := "pending_ws_heartbeat"
	if wsStats.ActiveConnections > 0 {
		traderStatus = "connected"
	}

	status := "ok"
	if dbComponent["status"] != "ok" || hsmTradingComponent["status"] != "ok" {
		status = "degraded"
	}

	response := gin.H{
		"status": status,
		"service": gin.H{
			"name":            h.serviceName,
			"version":         h.serviceVersion,
			"started_at_unix": h.startedAt.Unix(),
			"uptime_sec":      uptimeSec,
			"timestamp_unix":  now.Unix(),
		},
		"components": gin.H{
			"database":    dbComponent,
			"hsm_trading": hsmTradingComponent,
			"hsm_2fa":     hsmTwoFAComponent,
			"scheduler": gin.H{
				"status":               "ok",
				"cycle_count":          runtimeSnapshot.SchedulerCycleCount,
				"last_candidate_count": runtimeSnapshot.SchedulerLastCandidateCount,
				"last_run_unix":        runtimeSnapshot.SchedulerLastRunUnix,
				"assignment_mode":      "placeholder_noop",
			},
			"websocket": gin.H{
				"status":              "ok",
				"active_connections":  wsStats.ActiveConnections,
				"total_connections":   wsStats.TotalConnections,
				"last_connect_unix":   wsStats.LastConnectUnix,
				"last_heartbeat_unix": runtimeSnapshot.LastWSHeartbeatUnix,
				"last_timeout_unix":   runtimeSnapshot.LastWSTimeoutUnix,
				"timeout_count":       runtimeSnapshot.WSTimeoutCount,
			},
			"traders": gin.H{
				"status":                  traderStatus,
				"source":                  "ws_ping_pong",
				"aggregation_implemented": false,
				"connected_count":         wsStats.ActiveConnections,
				"last_heartbeat_unix":     runtimeSnapshot.LastWSHeartbeatUnix,
				"last_timeout_unix":       runtimeSnapshot.LastWSTimeoutUnix,
				"timeout_count":           runtimeSnapshot.WSTimeoutCount,
			},
		},
		"runtime": gin.H{
			"goroutines":      runtime.NumGoroutine(),
			"memory_alloc_mb": runtimeMem.Alloc / 1024 / 1024,
			"memory_sys_mb":   runtimeMem.Sys / 1024 / 1024,
			"gc_cycles":       runtimeMem.NumGC,
			"num_cpu":         runtime.NumCPU(),
		},
		"state": stateSnapshot,
	}

	if status != "ok" {
		c.JSON(http.StatusServiceUnavailable, response)
		return
	}

	c.JSON(http.StatusOK, response)
}

// Ready returns the readiness status (for Kubernetes)
// GET /ready
func (h *HealthHandler) Ready(c *gin.Context) {
	// For now, same as Health
	h.Health(c)
}

// Live returns the liveness status (for Kubernetes)
// GET /live
func (h *HealthHandler) Live(c *gin.Context) {
	// Simple liveness check - if we can respond, we're alive
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"timestamp": time.Now().Unix(),
	})
}

func (h *HealthHandler) checkDatabase() gin.H {
	if h.dbClient == nil {
		return gin.H{"status": "not_configured"}
	}

	started := time.Now()
	if err := h.dbClient.Ping(); err != nil {
		return gin.H{
			"status":     "error",
			"latency_ms": float64(time.Since(started).Microseconds()) / 1000.0,
			"error":      err.Error(),
		}
	}

	return gin.H{
		"status":     "ok",
		"latency_ms": float64(time.Since(started).Microseconds()) / 1000.0,
	}
}

func (h *HealthHandler) checkHSM(ctx context.Context, client *hsm.Client) gin.H {
	if client == nil {
		return gin.H{
			"status": "not_configured",
		}
	}

	started := time.Now()
	resp, err := client.Health(ctx)
	if err != nil {
		return gin.H{
			"status":     "error",
			"latency_ms": float64(time.Since(started).Microseconds()) / 1000.0,
			"error":      err.Error(),
		}
	}

	return gin.H{
		"status":        "ok",
		"latency_ms":    float64(time.Since(started).Microseconds()) / 1000.0,
		"hsm_status":    resp.Status,
		"hsm_available": resp.HSMAvailable,
		"kek_status":    resp.KEKStatus,
	}
}
