package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go-microservice/internal/middleware"
	"golang.org/x/time/rate"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestPerIPRateLimit_BurstThenTooManyRequests(t *testing.T) {
	lim := middleware.PerIPRateLimit(rate.Every(time.Hour), 3)
	r := gin.New()
	r.GET("/hit", lim, func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/hit", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: status %d", i, w.Code)
		}
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/hit", nil))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("4th request: got %d, want 429", w.Code)
	}
}

func TestPerIPRateLimit_ErrorResponseShape(t *testing.T) {
	lim := middleware.PerIPRateLimit(rate.Every(time.Hour), 1)
	r := gin.New()
	r.GET("/hit", lim, func(c *gin.Context) { c.Status(http.StatusOK) })

	// exhaust burst
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, httptest.NewRequest(http.MethodGet, "/hit", nil))
	if w1.Code != http.StatusOK {
		t.Fatal(w1.Code)
	}
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/hit", nil))
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %d", w2.Code)
	}
	body := w2.Body.String()
	if body == "" || body[0] != '{' {
		t.Errorf("expected JSON body, got %q", body)
	}
}
