package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"go-microservice/internal/observability"
)

func truncateUARunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// AccessLog emits one structured log line per request after it completes, including request_id,
// user_id when the route ran behind AuthRequired, method, matched route pattern, status, duration, client IP, and user-agent (truncated).
func AccessLog(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "(unmatched)"
		}
		status := c.Writer.Status()
		level := slog.LevelInfo
		switch {
		case status >= 500:
			level = slog.LevelError
		case status >= 400:
			level = slog.LevelWarn
		}

		ua := truncateUARunes(c.Request.UserAgent(), 512)
		attrs := []slog.Attr{
			slog.String("method", c.Request.Method),
			slog.String("path", route),
			slog.Int("status", status),
			slog.Duration("duration", time.Since(start)),
			slog.String("client_ip", c.ClientIP()),
			slog.String("user_agent", ua),
		}
		if rid, ok := c.Get(observability.RequestIDKey); ok {
			if s, ok := rid.(string); ok && s != "" {
				attrs = append(attrs, slog.String("request_id", s))
			}
		}
		if uid, ok := c.Get("userID"); ok {
			if s, ok := uid.(string); ok && s != "" {
				attrs = append(attrs, slog.String("user_id", s))
			}
		}

		log.LogAttrs(c.Request.Context(), level, "http_request", attrs...)
	}
}
