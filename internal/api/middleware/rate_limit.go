package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type ipRateWindow struct {
	count   int
	resetAt time.Time
}

// PerIPRateLimit applies a simple fixed-window rate limit by client IP.
func PerIPRateLimit(requestsPerSecond, burst int) gin.HandlerFunc {
	allowed := requestsPerSecond + burst
	if allowed <= 0 {
		allowed = 1
	}

	const window = time.Second
	var (
		mu      sync.Mutex
		windows = make(map[string]ipRateWindow)
	)

	return func(c *gin.Context) {
		ip := c.ClientIP()
		now := time.Now()

		mu.Lock()
		w := windows[ip]
		if w.resetAt.IsZero() || now.After(w.resetAt) {
			w = ipRateWindow{count: 0, resetAt: now.Add(window)}
		}
		w.count++
		windows[ip] = w
		remaining := allowed - w.count
		resetSec := int(w.resetAt.Sub(now).Seconds())
		mu.Unlock()

		if remaining < 0 {
			remaining = 0
		}
		if resetSec < 0 {
			resetSec = 0
		}

		c.Header("X-RateLimit-Limit", strconv.Itoa(allowed))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
		c.Header("X-RateLimit-Reset", strconv.Itoa(resetSec))

		if w.count > allowed {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded",
			})
			return
		}

		c.Next()
	}
}
