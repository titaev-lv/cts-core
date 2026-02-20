package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/titaev-lv/cts-core/internal/requestid"
)

const requestIDHeader = "X-Request-ID"

// RequestID ensures each request has a request_id and exposes it in context and response headers.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader(requestIDHeader)
		if requestID == "" {
			requestID = generateRequestID()
		}

		c.Set("request_id", requestID)
		c.Set("request_start", time.Now())
		ctx := requestid.WithContext(c.Request.Context(), requestID)
		if ctx != nil {
			c.Request = c.Request.WithContext(ctx)
		}
		c.Writer.Header().Set(requestIDHeader, requestID)
		c.Next()
	}
}

// GetRequestID returns request_id from gin.Context if present.
func GetRequestID(c *gin.Context) string {
	value, ok := c.Get("request_id")
	if !ok {
		return ""
	}
	requestID, _ := value.(string)
	return requestID
}

func GetRequestStart(c *gin.Context) (time.Time, bool) {
	value, ok := c.Get("request_start")
	if !ok {
		return time.Time{}, false
	}
	startedAt, ok := value.(time.Time)
	if !ok || startedAt.IsZero() {
		return time.Time{}, false
	}
	return startedAt, true
}

// PropagateRequestID copies request_id to outbound requests if set.
func PropagateRequestID(c *gin.Context, req *http.Request) {
	if req == nil {
		return
	}
	requestID := GetRequestID(c)
	if requestID == "" {
		return
	}
	req.Header.Set(requestIDHeader, requestID)
}

func generateRequestID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return hex.EncodeToString(buf)
}
