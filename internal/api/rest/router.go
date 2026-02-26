package rest

import (
	"github.com/gin-gonic/gin"
	"github.com/titaev-lv/cts-core/internal/api/middleware"
	"github.com/titaev-lv/cts-core/internal/api/ws"
	"github.com/titaev-lv/cts-core/internal/db"
)

// Options configures REST router behavior.
type Options struct {
	RESTRequestsPerSecond int
	RESTBurst             int
	WSRequestsPerSecond   int
	WSBurst               int
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

	healthHandler := NewHealthHandler(dbClient)
	rest := router.Group("/")
	rest.Use(middleware.PerIPRateLimit(opts.RESTRequestsPerSecond, opts.RESTBurst))
	rest.GET("/health", healthHandler.Health)
	rest.GET("/ready", healthHandler.Ready)
	rest.GET("/live", healthHandler.Live)

	wsHandler := ws.NewHandler()
	router.GET("/ws", middleware.PerIPRateLimit(opts.WSRequestsPerSecond, opts.WSBurst), wsHandler.Serve)

	return router
}
