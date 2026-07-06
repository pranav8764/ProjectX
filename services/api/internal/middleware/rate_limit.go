package middleware

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type rateLimitEntry struct {
	Count   int
	ResetAt time.Time
}

type slidingWindowLimiter struct {
	mu      sync.Mutex
	entries map[string]rateLimitEntry
	limit   int
	window  time.Duration
	now     func() time.Time
}

// RateLimitMiddleware applies a sliding window rate limit to the requests.
func RateLimitMiddleware(limit int, window time.Duration) gin.HandlerFunc {
	limiter := newSlidingWindowLimiter(limit, window)
	return func(c *gin.Context) {
		key := c.GetString("userID")
		if key == "" {
			key = c.ClientIP()
		}
		if key == "" {
			key = "unknown"
		}

		allowed, retryAfter := limiter.Allow(key)
		if !allowed {
			c.Header("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":           "too many requests, please retry shortly",
				"retryAfterSec":   int(retryAfter.Seconds()),
				"requestId":       c.GetString("requestID"),
				"rateLimitWindow": window.String(),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

func newSlidingWindowLimiter(limit int, window time.Duration) *slidingWindowLimiter {
	return &slidingWindowLimiter{
		entries: make(map[string]rateLimitEntry),
		limit:   limit,
		window:  window,
		now:     time.Now,
	}
}

func (l *slidingWindowLimiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	entry, ok := l.entries[key]
	if !ok || !now.Before(entry.ResetAt) {
		l.entries[key] = rateLimitEntry{Count: 1, ResetAt: now.Add(l.window)}
		l.cleanupExpired(now)
		return true, 0
	}

	if entry.Count >= l.limit {
		return false, entry.ResetAt.Sub(now)
	}

	entry.Count++
	l.entries[key] = entry
	return true, 0
}

func (l *slidingWindowLimiter) cleanupExpired(now time.Time) {
	if len(l.entries) < 1000 {
		return
	}
	for key, entry := range l.entries {
		if !now.Before(entry.ResetAt) {
			delete(l.entries, key)
		}
	}
}
