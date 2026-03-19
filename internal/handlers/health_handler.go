package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
)

type HealthHandler struct {
	mongo *mongo.Client
}

func NewHealthHandler(mongo *mongo.Client) *HealthHandler {
	return &HealthHandler{mongo: mongo}
}

// GET /health
func (h *HealthHandler) Health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	mongoStatus := "ok"
	if err := h.mongo.Ping(ctx, nil); err != nil {
		mongoStatus = "unreachable"
	}

	status := http.StatusOK
	if mongoStatus != "ok" {
		status = http.StatusServiceUnavailable
	}

	c.JSON(status, gin.H{
		"status":    map[bool]string{true: "healthy", false: "degraded"}[mongoStatus == "ok"],
		"timestamp": time.Now().UTC(),
		"checks": gin.H{
			"mongodb": mongoStatus,
		},
	})
}
