package middleware

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/titaev-lv/cts-core/internal/logger"
)

// AuditLog writes audit events for mutating requests.
func AuditLog() gin.HandlerFunc {
	log := logger.GetAudit("rest")
	return func(c *gin.Context) {
		method := c.Request.Method
		if !isMutatingMethod(method) {
			c.Next()
			return
		}

		start := time.Now()
		path := c.Request.URL.Path
		if rawQuery := c.Request.URL.RawQuery; rawQuery != "" {
			path = path + "?" + rawQuery
		}
		clientIP := c.ClientIP()
		userAgent := c.Request.UserAgent()
		requestID := GetRequestID(c)

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		success := status < 400

		if success {
			log.Info("audit", "method", method, "path", path, "status", status, "latency_ms", latency.Milliseconds(), "ip", clientIP, "user_agent", userAgent, "request_id", requestID)
			return
		}

		log.Warn("audit", "method", method, "path", path, "status", status, "latency_ms", latency.Milliseconds(), "ip", clientIP, "user_agent", userAgent, "request_id", requestID)
	}
}

func isMutatingMethod(method string) bool {
	switch strings.ToUpper(method) {
	case "POST", "PUT", "PATCH", "DELETE":
		return true
	default:
		return false
	}
}
