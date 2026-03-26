package observability

import (
	"log/slog"

	"github.com/gin-gonic/gin"
)

// LoggerFromGin returns a logger with request_id and user_id (when present) for use inside handlers.
func LoggerFromGin(c *gin.Context, base *slog.Logger) *slog.Logger {
	if base == nil {
		base = slog.Default()
	}
	var args []any
	if rid, ok := c.Get(RequestIDKey); ok {
		if s, ok := rid.(string); ok && s != "" {
			args = append(args, slog.String("request_id", s))
		}
	}
	if uid, ok := c.Get("userID"); ok {
		if s, ok := uid.(string); ok && s != "" {
			args = append(args, slog.String("user_id", s))
		}
	}
	if len(args) == 0 {
		return base
	}
	return base.With(args...)
}
