package rest

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/titaev-lv/cts-core/internal/db"
)

// HealthHandler handles health check requests
type HealthHandler struct {
	dbClient *db.MySQLClient
}

// NewHealthHandler creates a new health check handler
func NewHealthHandler(dbClient *db.MySQLClient) *HealthHandler {
	return &HealthHandler{
		dbClient: dbClient,
	}
}

// Health returns the health status of the service
// GET /health
func (h *HealthHandler) Health(c *gin.Context) {
	// Check database connection
	dbStatus := "ok"
	err := h.dbClient.Ping()
	if err != nil {
		dbStatus = "error: " + err.Error()
	}

	response := gin.H{
		"status": "ok",
		"components": gin.H{
			"database": dbStatus,
		},
		"timestamp": time.Now().Unix(),
	}

	// If any component is down, return 503
	if dbStatus != "ok" {
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
