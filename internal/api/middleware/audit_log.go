package middleware

import (
	"log/slog"
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
		latencyMS := float64(latency.Microseconds()) / 1000.0
		status := c.Writer.Status()
		success := status < 400

		fields := []any{
			"method", method,
			"path", path,
			"status", status,
			"latency_ms", latencyMS,
			"ip", clientIP,
			"user_agent", userAgent,
			"request_id", requestID,
		}

		if log.Enabled(c.Request.Context(), slog.LevelDebug) {
			if requestStart, ok := GetRequestStart(c); ok {
				requestTotalMS := float64(time.Since(requestStart).Microseconds()) / 1000.0
				beforeMiddlewareMS := float64(start.Sub(requestStart).Microseconds()) / 1000.0
				if beforeMiddlewareMS < 0 {
					beforeMiddlewareMS = 0
				}
				outsideScopeMS := requestTotalMS - beforeMiddlewareMS - latencyMS
				if outsideScopeMS < 0 {
					outsideScopeMS = 0
				}

				fields = append(fields, "latency_breakdown_ms", map[string]float64{
					"total_latency_ms":       requestTotalMS,
					"request_handler_ms":     latencyMS,
					"middleware_overhead_ms": beforeMiddlewareMS + outsideScopeMS,
				})
			}
		}

		if success {
			log.Info("audit", fields...)
			return
		}

		log.Warn("audit", fields...)
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
