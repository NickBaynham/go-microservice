package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go-microservice/internal/middleware"
	"go-microservice/internal/observability"
)

func TestRequestID_GeneratesAndEchoes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.ServeHTTP(w, req)

	if w.Header().Get("X-Request-ID") == "" {
		t.Fatal("expected X-Request-ID response header")
	}
}

func TestRequestID_PassthroughFromClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.GET("/x", func(c *gin.Context) {
		v, _ := c.Get(observability.RequestIDKey)
		s, _ := v.(string)
		if s != "upstream-abc" {
			t.Errorf("context request_id: got %q", s)
		}
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Request-ID", "upstream-abc")
	r.ServeHTTP(w, req)

	if w.Header().Get("X-Request-ID") != "upstream-abc" {
		t.Errorf("response X-Request-ID: got %q", w.Header().Get("X-Request-ID"))
	}
}
