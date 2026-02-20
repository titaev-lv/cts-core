package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/titaev-lv/cts-core/internal/logger"
)

// AccessLog writes structured access logs to access.log.
func AccessLog() gin.HandlerFunc {
	log := logger.GetAccess("rest")
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		if rawQuery := c.Request.URL.RawQuery; rawQuery != "" {
			path = path + "?" + rawQuery
		}

		method := c.Request.Method
		clientIP := c.ClientIP()
		userAgent := c.Request.UserAgent()
		requestID := GetRequestID(c)

		c.Next()

		latency := time.Since(start)
		latencyMS := float64(latency.Microseconds()) / 1000.0
		status := c.Writer.Status()
		size := c.Writer.Size()

		log.Info("HTTP access",
			"method", method,
			"path", path,
			"status", status,
			"latency_ms", latencyMS,
			"ip", clientIP,
			"user_agent", userAgent,
			"bytes", size,
			"request_id", requestID,
		)
	}
}
