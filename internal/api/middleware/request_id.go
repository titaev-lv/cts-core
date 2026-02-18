package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/gin-gonic/gin"
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
