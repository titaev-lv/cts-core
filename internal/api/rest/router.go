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
	WSHandler             *ws.Handler
	WSHeartbeatInterval   time.Duration
	WSHeartbeatTimeout    time.Duration
	WSWriteTimeout        time.Duration
	WSMaxPayloadBytes     int
	WSMaxMessagesPerSec   int
	WSMaxUnknownActions   int
	WSUnknownActionWindow time.Duration
	WSRequestDedupWindow  time.Duration
	WSAllowedCommonNames  []string
	WSAllowedOUs          []string
	WSAllowedDNSNames     []string
	HSMTrading            *hsm.Client
	HSMTwoFA              *hsm.Client
	StateManager          *state.Manager
	StartedAt             time.Time
	ServiceVersion        string
	MetricsEnabled        bool
	MetricsPath           string
}

// NewRouter configures REST routes and middleware.
func NewRouter(dbClient *db.MySQLClient, opts Options) (*gin.Engine, *ws.Handler) {
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	router.Use(
		middleware.RequestID(),
		middleware.Recovery(),
		middleware.AccessLog(),
		middleware.ErrorLog(),
		middleware.AuditLog(),
	)

	wsHandler := opts.WSHandler
	if wsHandler == nil {
		wsHandler = ws.NewHandlerWithOptions(ws.HandlerOptions{
			HeartbeatInterval:   opts.WSHeartbeatInterval,
			HeartbeatTimeout:    opts.WSHeartbeatTimeout,
			WriteTimeout:        opts.WSWriteTimeout,
			MaxPayloadBytes:     opts.WSMaxPayloadBytes,
			MaxMessagesPerSec:   opts.WSMaxMessagesPerSec,
			MaxUnknownActions:   opts.WSMaxUnknownActions,
			UnknownActionWindow: opts.WSUnknownActionWindow,
			RequestDedupWindow:  opts.WSRequestDedupWindow,
			RequireClientCert:   true,
			AllowedCommonNames:  opts.WSAllowedCommonNames,
			AllowedOUs:          opts.WSAllowedOUs,
			AllowedDNSNames:     opts.WSAllowedDNSNames,
			Persistence:         newWSSessionPersistence(dbClient),
			StateManager:        opts.StateManager,
		})
	}
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
	if opts.MetricsEnabled {
		rest.GET(normalizeMetricsPath(opts.MetricsPath), newMetricsHandler(MetricsHandlerOptions{
			WSHandler:    wsHandler,
			StateManager: opts.StateManager,
		}))
	}

	router.GET("/ws", middleware.PerIPRateLimit(opts.WSRequestsPerSecond, opts.WSBurst), wsHandler.Serve)

	return router, wsHandler
}
