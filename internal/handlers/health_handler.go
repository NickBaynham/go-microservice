package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go-microservice/internal/models"
	"go.mongodb.org/mongo-driver/mongo"
)

type HealthHandler struct {
	mongo *mongo.Client
}

func NewHealthHandler(mongo *mongo.Client) *HealthHandler {
	return &HealthHandler{mongo: mongo}
}

// Health godoc
// @Summary      Health check
// @Description  Returns the health status of the service and its dependencies.
// @Tags         system
// @Produce      json
// @Success      200  {object}  models.HealthResponse  "Service is healthy"
// @Failure      503  {object}  models.HealthResponse  "Service is degraded"
// @Router       /health [get]
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

	c.JSON(status, models.HealthResponse{
		Status:    map[bool]string{true: "healthy", false: "degraded"}[mongoStatus == "ok"],
		Timestamp: time.Now().UTC(),
		Checks:    map[string]string{"mongodb": mongoStatus},
	})
}
