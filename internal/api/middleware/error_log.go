package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/titaev-lv/cts-core/internal/logger"
)

// ErrorLog writes handler errors to error.log.
func ErrorLog() gin.HandlerFunc {
	log := logger.Get("rest")
	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) == 0 {
			return
		}

		requestID := GetRequestID(c)
		for _, err := range c.Errors {
			log.Error("HTTP handler error",
				"error", err.Err,
				"type", err.Type,
				"meta", err.Meta,
				"method", c.Request.Method,
				"path", c.Request.URL.Path,
				"status", c.Writer.Status(),
				"request_id", requestID,
			)
		}
	}
}
