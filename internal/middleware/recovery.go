package middleware

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"go-microservice/internal/models"
	"go-microservice/internal/observability"
)

// SlogRecovery logs panics with request context and returns 500 JSON. It should run as the outermost
// Gin middleware so inner layers (e.g. RequestID) have already populated the context.
func SlogRecovery(log *slog.Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, err any) {
		attrs := []any{
			slog.Any("panic", err),
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
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
		log.Error("panic recovered", attrs...)
		c.AbortWithStatusJSON(http.StatusInternalServerError, models.ErrorResponse{Error: "internal server error"})
	})
}
