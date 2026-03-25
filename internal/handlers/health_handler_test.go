package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go-microservice/internal/handlers"
	"go-microservice/internal/models"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

type errPinger struct{}

func (errPinger) Ping(ctx context.Context, rp *readpref.ReadPref) error {
	return errors.New("mongo unreachable")
}

type okPinger struct{}

func (okPinger) Ping(ctx context.Context, rp *readpref.ReadPref) error {
	return nil
}

func TestHealth_MongoOK_Returns200(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := handlers.NewHealthHandler(okPinger{})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	h.Health(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusOK)
	}
	var body models.HealthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "healthy" {
		t.Errorf("status field: got %q, want healthy", body.Status)
	}
	if body.Checks["mongodb"] != "ok" {
		t.Errorf("checks.mongodb: got %q, want ok", body.Checks["mongodb"])
	}
}

func TestHealth_MongoDown_Returns503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := handlers.NewHealthHandler(errPinger{})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	h.Health(c)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
	var body models.HealthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "degraded" {
		t.Errorf("status field: got %q, want degraded", body.Status)
	}
	if body.Checks["mongodb"] != "unreachable" {
		t.Errorf("checks.mongodb: got %q, want unreachable", body.Checks["mongodb"])
	}
}
