package rest

import (
	"github.com/gin-gonic/gin"
	"github.com/titaev-lv/cts-core/internal/api/middleware"
	"github.com/titaev-lv/cts-core/internal/api/ws"
	"github.com/titaev-lv/cts-core/internal/db"
)

// Options configures REST router behavior.
type Options struct {
	WSDebug bool
}

// NewRouter configures REST routes and middleware.
func NewRouter(dbClient *db.MySQLClient, opts Options) *gin.Engine {
	router := gin.New()
	router.Use(
		middleware.RequestID(),
		middleware.Recovery(),
		middleware.AccessLog(),
		middleware.ErrorLog(),
		middleware.AuditLog(),
	)

	healthHandler := NewHealthHandler(dbClient)
	router.GET("/health", healthHandler.Health)
	router.GET("/ready", healthHandler.Ready)
	router.GET("/live", healthHandler.Live)

	wsHandler := ws.NewHandler(opts.WSDebug)
	router.GET("/ws", wsHandler.Serve)

	return router
}
