package rest

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/titaev-lv/cts-core/internal/api/middleware"
	"github.com/titaev-lv/cts-core/internal/api/ws"
	"github.com/titaev-lv/cts-core/internal/db"
	"github.com/titaev-lv/cts-core/internal/hsm"
	"github.com/titaev-lv/cts-core/internal/state"
)

// Options configures REST router behavior.
type Options struct {
	RESTRequestsPerSecond int
	RESTBurst             int
	WSRequestsPerSecond   int
	WSBurst               int
	WSHeartbeatInterval   time.Duration
	WSHeartbeatTimeout    time.Duration
	HSMTrading            *hsm.Client
	HSMTwoFA              *hsm.Client
	StateManager          *state.Manager
	StartedAt             time.Time
	ServiceVersion        string
}

// NewRouter configures REST routes and middleware.
func NewRouter(dbClient *db.MySQLClient, opts Options) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	router.Use(
		middleware.RequestID(),
		middleware.Recovery(),
		middleware.AccessLog(),
		middleware.ErrorLog(),
		middleware.AuditLog(),
	)

	wsHandler := ws.NewHandlerWithOptions(ws.HandlerOptions{
		HeartbeatInterval: opts.WSHeartbeatInterval,
		HeartbeatTimeout:  opts.WSHeartbeatTimeout,
		StateManager:      opts.StateManager,
	})
	healthHandler := NewHealthHandler(dbClient, HealthHandlerOptions{
		HSMTrading:     opts.HSMTrading,
		HSMTwoFA:       opts.HSMTwoFA,
		StateManager:   opts.StateManager,
		WSHandler:      wsHandler,
		StartedAt:      opts.StartedAt,
		ServiceName:    "cts-core",
		ServiceVersion: opts.ServiceVersion,
	})
	rest := router.Group("/")
	rest.Use(middleware.PerIPRateLimit(opts.RESTRequestsPerSecond, opts.RESTBurst))
	rest.GET("/health", healthHandler.Health)
	rest.GET("/ready", healthHandler.Ready)
	rest.GET("/live", healthHandler.Live)

	router.GET("/ws", middleware.PerIPRateLimit(opts.WSRequestsPerSecond, opts.WSBurst), wsHandler.Serve)

	return router
}
