package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"github.com/titaev-lv/cts-core/internal/logger"
)

// Recovery handles panics and logs them to error.log.
func Recovery() gin.HandlerFunc {
	log := logger.Get("rest")
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				requestID := GetRequestID(c)
				log.Error("panic recovered",
					"error", recovered,
					"method", c.Request.Method,
					"path", c.Request.URL.Path,
					"request_id", requestID,
					"stack", string(debug.Stack()),
				)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": "internal server error",
				})
			}
		}()

		c.Next()
	}
}
