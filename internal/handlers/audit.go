package handlers

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"go-microservice/internal/observability"
)

const maxUserAgentRunes = 512

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// AuthAudit emits a structured event for security-sensitive auth actions (correlate with request_id in access logs).
func AuthAudit(c *gin.Context, level slog.Level, event string, attrs ...slog.Attr) {
	base := []slog.Attr{
		slog.String("event", event),
		slog.String("client_ip", c.ClientIP()),
		slog.String("user_agent", truncateRunes(c.Request.UserAgent(), maxUserAgentRunes)),
	}
	if rid, ok := c.Get(observability.RequestIDKey); ok {
		if s, ok := rid.(string); ok && s != "" {
			base = append(base, slog.String("request_id", s))
		}
	}
	base = append(base, attrs...)
	slog.LogAttrs(c.Request.Context(), level, "auth_audit", base...)
}
