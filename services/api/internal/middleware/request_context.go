package middleware

import (
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequestContextMiddleware adds a unique request ID to the request context and logs request details.
func RequestContextMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := strings.TrimSpace(c.GetHeader("X-Request-ID"))
		if requestID == "" || len(requestID) > 128 {
			requestID = uuid.NewString()
		}

		start := time.Now()
		c.Set("requestID", requestID)
		c.Writer.Header().Set("X-Request-ID", requestID)

		c.Next()

		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		log.Printf(
			"request_id=%s method=%s path=%s status=%d latency_ms=%d user_id=%s org_id=%s role=%s",
			requestID,
			c.Request.Method,
			path,
			c.Writer.Status(),
			time.Since(start).Milliseconds(),
			c.GetString("userID"),
			c.GetString("orgID"),
			c.GetString("role"),
		)
	}
}
