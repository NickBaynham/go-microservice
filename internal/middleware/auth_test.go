package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go-microservice/internal/auth"
	"go-microservice/internal/middleware"
)

const testSecret = "middleware-test-secret"

func init() {
	gin.SetMode(gin.TestMode)
}

// newRouter builds a test router with the given middleware and a simple OK handler.
func newRouter(mw ...gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	r.GET("/test", append(mw, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})...)
	return r
}

func makeToken(t *testing.T, userID, email, role string) string {
	t.Helper()
	token, err := auth.GenerateToken(userID, email, role, testSecret, "1")
	if err != nil {
		t.Fatalf("makeToken: %v", err)
	}
	return token
}

// ── AuthRequired ──────────────────────────────────────────────────────────────

func TestAuthRequired_ValidToken_Passes(t *testing.T) {
	r := newRouter(middleware.AuthRequired(testSecret))
	token := makeToken(t, "user1", "alice@example.com", "user")

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAuthRequired_MissingHeader_Returns401(t *testing.T) {
	r := newRouter(middleware.AuthRequired(testSecret))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthRequired_NoBearerPrefix_Returns401(t *testing.T) {
	r := newRouter(middleware.AuthRequired(testSecret))
	token := makeToken(t, "user1", "alice@example.com", "user")

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", token) // missing "Bearer " prefix
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthRequired_InvalidToken_Returns401(t *testing.T) {
	r := newRouter(middleware.AuthRequired(testSecret))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer this.is.invalid")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthRequired_WrongSecret_Returns401(t *testing.T) {
	r := newRouter(middleware.AuthRequired(testSecret))

	// Token signed with a different secret
	token, _ := auth.GenerateToken("user1", "alice@example.com", "user", "different-secret", "1")

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthRequired_SetsClaimsInContext(t *testing.T) {
	var gotUserID, gotEmail, gotRole string

	r := gin.New()
	r.GET("/test", middleware.AuthRequired(testSecret), func(c *gin.Context) {
		if v, ok := c.Get("userID"); ok {
			gotUserID, _ = v.(string)
		}
		if v, ok := c.Get("email"); ok {
			gotEmail, _ = v.(string)
		}
		if v, ok := c.Get("role"); ok {
			gotRole, _ = v.(string)
		}
		c.Status(http.StatusOK)
	})

	token := makeToken(t, "user99", "ctx@example.com", "admin")
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if gotUserID != "user99" {
		t.Errorf("userID: got %q, want %q", gotUserID, "user99")
	}
	if gotEmail != "ctx@example.com" {
		t.Errorf("email: got %q, want %q", gotEmail, "ctx@example.com")
	}
	if gotRole != "admin" {
		t.Errorf("role: got %q, want %q", gotRole, "admin")
	}
}

// ── AdminOnly ─────────────────────────────────────────────────────────────────

func TestAdminOnly_AdminRole_Passes(t *testing.T) {
	r := gin.New()
	r.GET("/test", func(c *gin.Context) {
		c.Set("role", "admin")
		c.Next()
	}, middleware.AdminOnly(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAdminOnly_UserRole_Returns403(t *testing.T) {
	r := gin.New()
	r.GET("/test", func(c *gin.Context) {
		c.Set("role", "user")
		c.Next()
	}, middleware.AdminOnly(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestAdminOnly_NoRole_Returns403(t *testing.T) {
	r := newRouter(middleware.AdminOnly())

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestAdminOnly_FullStack_AdminToken_Passes(t *testing.T) {
	r := newRouter(middleware.AuthRequired(testSecret), middleware.AdminOnly())
	token := makeToken(t, "admin1", "admin@example.com", "admin")

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAdminOnly_FullStack_UserToken_Returns403(t *testing.T) {
	r := newRouter(middleware.AuthRequired(testSecret), middleware.AdminOnly())
	token := makeToken(t, "user1", "user@example.com", "user")

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusForbidden)
	}
}
