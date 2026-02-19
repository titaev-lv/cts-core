package rest

import (
	"github.com/gin-gonic/gin"
	"github.com/titaev-lv/cts-core/internal/api/middleware"
	"github.com/titaev-lv/cts-core/internal/db"
)

// NewRouter configures REST routes and middleware.
func NewRouter(dbClient *db.MySQLClient) *gin.Engine {
	router := gin.New()
	router.Use(
		middleware.RequestID(),
		middleware.AccessLog(),
		middleware.ErrorLog(),
		middleware.Recovery(),
	)

	healthHandler := NewHealthHandler(dbClient)
	router.GET("/health", healthHandler.Health)
	router.GET("/ready", healthHandler.Ready)
	router.GET("/live", healthHandler.Live)

	return router
}
