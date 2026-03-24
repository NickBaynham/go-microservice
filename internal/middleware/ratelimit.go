package middleware

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"go-microservice/internal/models"
	"golang.org/x/time/rate"
)

// PerIPRateLimit limits requests per client IP using a token bucket.
func PerIPRateLimit(r rate.Limit, burst int) gin.HandlerFunc {
	var mu sync.Mutex
	limiters := make(map[string]*rate.Limiter)

	return func(c *gin.Context) {
		ip := c.ClientIP()

		mu.Lock()
		lim, ok := limiters[ip]
		if !ok {
			lim = rate.NewLimiter(r, burst)
			limiters[ip] = lim
		}
		mu.Unlock()

		if !lim.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, models.ErrorResponse{Error: "too many requests"})
			return
		}
		c.Next()
	}
}
