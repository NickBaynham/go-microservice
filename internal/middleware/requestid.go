package middleware

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/gin-gonic/gin"
	"go-microservice/internal/observability"
)

const headerRequestID = "X-Request-ID"

// RequestID ensures every request has a correlation ID: uses X-Request-ID from the client when present,
// otherwise generates one. The value is stored in the gin context (observability.RequestIDKey) and echoed
// on the response.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader(headerRequestID)
		if rid == "" {
			rid = randomID()
		}
		c.Set(observability.RequestIDKey, rid)
		c.Header(headerRequestID, rid)
		c.Next()
	}
}

func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b[:])
}
