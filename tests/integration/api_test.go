// Package integration contains end-to-end tests for the go-microservice API.
//
// Tests run against a live server (local or Docker). Configure via environment:
//
//	TEST_HOST            default: localhost
//	TEST_PORT            default: 8443
//	TEST_SCHEME          default: https
//	TEST_SKIP_TLS_VERIFY default: true  (set false for production certs)
//
// Run with: make test-integration
package integration

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ── Fixtures ──────────────────────────────────────────────────────────────────

const (
	adminEmail    = "admin@test.com"
	adminPassword = "adminpass123"
	adminName     = "Test Admin"

	userEmail    = "user@test.com"
	userPassword = "userpass123"
	userName     = "Test User"
)

// ── DB cleanup ────────────────────────────────────────────────────────────────

// cleanDB drops the users collection before and after each test run
// to ensure a clean, predictable state.
func cleanDB(t *testing.T) {
	t.Helper()
	mongoURI := getEnv("MONGO_URI", "mongodb://localhost:27017")
	mongoDB := getEnv("MONGO_DB", "userservice")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		t.Fatalf("cleanDB: connect to mongo: %v", err)
	}
	defer client.Disconnect(ctx) //nolint:errcheck

	if err := client.Ping(ctx, nil); err != nil {
		t.Fatalf("cleanDB: ping mongo: %v", err)
	}

	_, err = client.Database(mongoDB).Collection("users").DeleteMany(ctx, bson.M{})
	if err != nil {
		t.Fatalf("cleanDB: delete users: %v", err)
	}
	t.Log("✔ database cleaned")
}

// ── Wait for server ───────────────────────────────────────────────────────────

// waitForServer polls the health endpoint until the server is ready or times out.
func waitForServer(t *testing.T, cfg testConfig, client *http.Client) {
	t.Helper()
	url := cfg.BaseURL + "/health"
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url) //nolint:noctx
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			t.Log("✔ server is ready")
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("server did not become ready within 30 seconds")
}

// ── Main test suite ───────────────────────────────────────────────────────────

func TestAPI(t *testing.T) {
	cfg := loadConfig()
	client := newClient()

	// Wait for server to be healthy before running any tests
	waitForServer(t, cfg, client)

	// Clean database before suite
	cleanDB(t)

	// Clean database after suite (deferred so it runs even on failure)
	t.Cleanup(func() { cleanDB(t) })

	// State shared between sub-tests (tokens, IDs)
	var (
		adminToken string
		userToken  string
		adminID    string
		userID     string
	)

	// ── Health ────────────────────────────────────────────────────────────────

	t.Run("GET /health returns healthy", func(t *testing.T) {
		r := do(t, client, http.MethodGet, cfg.BaseURL+"/health", "", nil)

		assertStatus(t, r.StatusCode, http.StatusOK, r)
		assertStringField(t, r, "status", "healthy")
		assertKey(t, r, "timestamp")

		checks := assertKey(t, r, "checks")
		if checks == nil {
			return
		}
		checksMap, ok := checks.(map[string]any)
		if !ok {
			t.Errorf("checks: expected object, got %T", checks)
			return
		}
		if checksMap["mongodb"] != "ok" {
			t.Errorf("checks.mongodb: got %v, want \"ok\"", checksMap["mongodb"])
		}
	})

	// ── Registration ─────────────────────────────────────────────────────────

	t.Run("POST /auth/register creates admin user", func(t *testing.T) {
		r := do(t, client, http.MethodPost, cfg.BaseURL+"/auth/register", "", map[string]any{
			"name":     adminName,
			"email":    adminEmail,
			"password": adminPassword,
			"role":     "admin",
		})

		assertStatus(t, r.StatusCode, http.StatusCreated, r)
		assertStringField(t, r, "message", "user registered successfully")
		assertNestedStringField(t, r, "user", "name", adminName)
		assertNestedStringField(t, r, "user", "email", adminEmail)
		assertNestedStringField(t, r, "user", "role", "admin")

		// Password must never be returned
		if user, ok := r.Body["user"].(map[string]any); ok {
			if _, hasPassword := user["password"]; hasPassword {
				t.Error("password field must not be returned in response")
			}
		}

		adminID = extractNestedString(r, "user", "id")
		if adminID == "" {
			t.Error("expected non-empty user id in response")
		}
	})

	t.Run("POST /auth/register creates regular user", func(t *testing.T) {
		r := do(t, client, http.MethodPost, cfg.BaseURL+"/auth/register", "", map[string]any{
			"name":     userName,
			"email":    userEmail,
			"password": userPassword,
		})

		assertStatus(t, r.StatusCode, http.StatusCreated, r)
		assertNestedStringField(t, r, "user", "role", "user")

		userID = extractNestedString(r, "user", "id")
		if userID == "" {
			t.Error("expected non-empty user id in response")
		}
	})

	t.Run("POST /auth/register rejects duplicate email", func(t *testing.T) {
		r := do(t, client, http.MethodPost, cfg.BaseURL+"/auth/register", "", map[string]any{
			"name":     "Duplicate",
			"email":    adminEmail,
			"password": "somepassword",
		})

		assertStatus(t, r.StatusCode, http.StatusConflict, r)
		assertKey(t, r, "error")
	})

	t.Run("POST /auth/register rejects missing fields", func(t *testing.T) {
		r := do(t, client, http.MethodPost, cfg.BaseURL+"/auth/register", "", map[string]any{
			"name": "No Email",
		})

		assertStatus(t, r.StatusCode, http.StatusBadRequest, r)
		assertKey(t, r, "error")
	})

	t.Run("POST /auth/register rejects short password", func(t *testing.T) {
		r := do(t, client, http.MethodPost, cfg.BaseURL+"/auth/register", "", map[string]any{
			"name":     "Short Pass",
			"email":    "short@test.com",
			"password": "123",
		})

		assertStatus(t, r.StatusCode, http.StatusBadRequest, r)
		assertKey(t, r, "error")
	})

	// ── Login ─────────────────────────────────────────────────────────────────

	t.Run("POST /auth/login returns token for admin", func(t *testing.T) {
		r := do(t, client, http.MethodPost, cfg.BaseURL+"/auth/login", "", map[string]any{
			"email":    adminEmail,
			"password": adminPassword,
		})

		assertStatus(t, r.StatusCode, http.StatusOK, r)
		adminToken = assertNonEmptyString(t, r, "token")
		assertNestedStringField(t, r, "user", "email", adminEmail)
		assertNestedStringField(t, r, "user", "role", "admin")

		// Password must never be returned
		if user, ok := r.Body["user"].(map[string]any); ok {
			if _, hasPassword := user["password"]; hasPassword {
				t.Error("password field must not be returned in login response")
			}
		}
	})

	t.Run("POST /auth/login returns token for user", func(t *testing.T) {
		r := do(t, client, http.MethodPost, cfg.BaseURL+"/auth/login", "", map[string]any{
			"email":    userEmail,
			"password": userPassword,
		})

		assertStatus(t, r.StatusCode, http.StatusOK, r)
		userToken = assertNonEmptyString(t, r, "token")
		assertNestedStringField(t, r, "user", "email", userEmail)
	})

	t.Run("POST /auth/login rejects wrong password", func(t *testing.T) {
		r := do(t, client, http.MethodPost, cfg.BaseURL+"/auth/login", "", map[string]any{
			"email":    adminEmail,
			"password": "wrongpassword",
		})

		assertStatus(t, r.StatusCode, http.StatusUnauthorized, r)
		assertKey(t, r, "error")
	})

	t.Run("POST /auth/login rejects unknown email", func(t *testing.T) {
		r := do(t, client, http.MethodPost, cfg.BaseURL+"/auth/login", "", map[string]any{
			"email":    "nobody@test.com",
			"password": "somepassword",
		})

		assertStatus(t, r.StatusCode, http.StatusUnauthorized, r)
		assertKey(t, r, "error")
	})

	// ── GET /me ───────────────────────────────────────────────────────────────

	t.Run("GET /me returns current user", func(t *testing.T) {
		r := do(t, client, http.MethodGet, cfg.BaseURL+"/me", adminToken, nil)

		assertStatus(t, r.StatusCode, http.StatusOK, r)
		assertStringField(t, r, "email", adminEmail)
		assertStringField(t, r, "name", adminName)
		assertStringField(t, r, "role", "admin")
		assertKey(t, r, "id")
		assertKey(t, r, "created_at")
		assertKey(t, r, "updated_at")
	})

	t.Run("GET /me rejects unauthenticated request", func(t *testing.T) {
		r := do(t, client, http.MethodGet, cfg.BaseURL+"/me", "", nil)

		assertStatus(t, r.StatusCode, http.StatusUnauthorized, r)
		assertKey(t, r, "error")
	})

	t.Run("GET /me rejects invalid token", func(t *testing.T) {
		r := do(t, client, http.MethodGet, cfg.BaseURL+"/me", "invalid.token.here", nil)

		assertStatus(t, r.StatusCode, http.StatusUnauthorized, r)
		assertKey(t, r, "error")
	})

	// ── GET /users (admin only) ───────────────────────────────────────────────

	t.Run("GET /users returns all users for admin", func(t *testing.T) {
		r := do(t, client, http.MethodGet, cfg.BaseURL+"/users", adminToken, nil)

		assertStatus(t, r.StatusCode, http.StatusOK, r)
		assertKey(t, r, "users")

		count, ok := r.Body["count"].(float64)
		if !ok {
			t.Errorf("count: expected number, got %T", r.Body["count"])
			return
		}
		if int(count) < 2 {
			t.Errorf("count: expected at least 2 users, got %d", int(count))
		}
	})

	t.Run("GET /users rejects regular user", func(t *testing.T) {
		r := do(t, client, http.MethodGet, cfg.BaseURL+"/users", userToken, nil)

		assertStatus(t, r.StatusCode, http.StatusForbidden, r)
		assertKey(t, r, "error")
	})

	t.Run("GET /users rejects unauthenticated request", func(t *testing.T) {
		r := do(t, client, http.MethodGet, cfg.BaseURL+"/users", "", nil)

		assertStatus(t, r.StatusCode, http.StatusUnauthorized, r)
		assertKey(t, r, "error")
	})

	// ── GET /users/:id (admin only) ───────────────────────────────────────────

	t.Run("GET /users/:id returns user for admin", func(t *testing.T) {
		r := do(t, client, http.MethodGet, fmt.Sprintf("%s/users/%s", cfg.BaseURL, userID), adminToken, nil)

		assertStatus(t, r.StatusCode, http.StatusOK, r)
		assertStringField(t, r, "email", userEmail)
		assertStringField(t, r, "name", userName)
		assertStringField(t, r, "role", "user")
	})

	t.Run("GET /users/:id returns 404 for unknown id", func(t *testing.T) {
		r := do(t, client, http.MethodGet, cfg.BaseURL+"/users/000000000000000000000000", adminToken, nil)

		assertStatus(t, r.StatusCode, http.StatusNotFound, r)
		assertKey(t, r, "error")
	})

	t.Run("GET /users/:id rejects regular user", func(t *testing.T) {
		r := do(t, client, http.MethodGet, fmt.Sprintf("%s/users/%s", cfg.BaseURL, adminID), userToken, nil)

		assertStatus(t, r.StatusCode, http.StatusForbidden, r)
		assertKey(t, r, "error")
	})

	// ── PUT /users/:id ────────────────────────────────────────────────────────

	t.Run("PUT /users/:id user can update own profile", func(t *testing.T) {
		r := do(t, client, http.MethodPut, fmt.Sprintf("%s/users/%s", cfg.BaseURL, userID), userToken, map[string]any{
			"name": "Updated User Name",
		})

		assertStatus(t, r.StatusCode, http.StatusOK, r)
		assertStringField(t, r, "name", "Updated User Name")
		assertStringField(t, r, "email", userEmail)
	})

	t.Run("PUT /users/:id user cannot update another user", func(t *testing.T) {
		r := do(t, client, http.MethodPut, fmt.Sprintf("%s/users/%s", cfg.BaseURL, adminID), userToken, map[string]any{
			"name": "Hacked Name",
		})

		assertStatus(t, r.StatusCode, http.StatusForbidden, r)
		assertKey(t, r, "error")
	})

	t.Run("PUT /users/:id admin can update any user", func(t *testing.T) {
		r := do(t, client, http.MethodPut, fmt.Sprintf("%s/users/%s", cfg.BaseURL, userID), adminToken, map[string]any{
			"name": "Admin Updated Name",
		})

		assertStatus(t, r.StatusCode, http.StatusOK, r)
		assertStringField(t, r, "name", "Admin Updated Name")
	})

	t.Run("PUT /users/:id admin can change user role", func(t *testing.T) {
		r := do(t, client, http.MethodPut, fmt.Sprintf("%s/users/%s", cfg.BaseURL, userID), adminToken, map[string]any{
			"role": "admin",
		})

		assertStatus(t, r.StatusCode, http.StatusOK, r)
		assertStringField(t, r, "role", "admin")

		// Reset role back to user
		do(t, client, http.MethodPut, fmt.Sprintf("%s/users/%s", cfg.BaseURL, userID), adminToken, map[string]any{
			"role": "user",
		})
	})

	t.Run("PUT /users/:id rejects empty update body", func(t *testing.T) {
		r := do(t, client, http.MethodPut, fmt.Sprintf("%s/users/%s", cfg.BaseURL, adminID), adminToken, map[string]any{})

		assertStatus(t, r.StatusCode, http.StatusBadRequest, r)
		assertKey(t, r, "error")
	})

	t.Run("PUT /users/:id rejects unauthenticated request", func(t *testing.T) {
		r := do(t, client, http.MethodPut, fmt.Sprintf("%s/users/%s", cfg.BaseURL, userID), "", map[string]any{
			"name": "Ghost",
		})

		assertStatus(t, r.StatusCode, http.StatusUnauthorized, r)
		assertKey(t, r, "error")
	})

	// ── DELETE /users/:id (admin only) ────────────────────────────────────────

	t.Run("DELETE /users/:id rejects regular user", func(t *testing.T) {
		r := do(t, client, http.MethodDelete, fmt.Sprintf("%s/users/%s", cfg.BaseURL, adminID), userToken, nil)

		assertStatus(t, r.StatusCode, http.StatusForbidden, r)
		assertKey(t, r, "error")
	})

	t.Run("DELETE /users/:id rejects unauthenticated request", func(t *testing.T) {
		r := do(t, client, http.MethodDelete, fmt.Sprintf("%s/users/%s", cfg.BaseURL, userID), "", nil)

		assertStatus(t, r.StatusCode, http.StatusUnauthorized, r)
		assertKey(t, r, "error")
	})

	t.Run("DELETE /users/:id admin can delete user", func(t *testing.T) {
		// Create a throwaway user to delete
		reg := do(t, client, http.MethodPost, cfg.BaseURL+"/auth/register", "", map[string]any{
			"name":     "Throwaway User",
			"email":    "throwaway@test.com",
			"password": "throwaway123",
		})
		assertStatus(t, reg.StatusCode, http.StatusCreated, reg)
		throwawayID := extractNestedString(reg, "user", "id")

		r := do(t, client, http.MethodDelete, fmt.Sprintf("%s/users/%s", cfg.BaseURL, throwawayID), adminToken, nil)

		assertStatus(t, r.StatusCode, http.StatusOK, r)
		assertStringField(t, r, "message", "user deleted successfully")
	})

	t.Run("DELETE /users/:id returns 404 for unknown id", func(t *testing.T) {
		r := do(t, client, http.MethodDelete, cfg.BaseURL+"/users/000000000000000000000000", adminToken, nil)

		assertStatus(t, r.StatusCode, http.StatusNotFound, r)
		assertKey(t, r, "error")
	})

	// Verify deleted user is actually gone
	t.Run("GET /users/:id returns 404 after deletion", func(t *testing.T) {
		// Re-register and delete a user, then verify it's gone
		reg := do(t, client, http.MethodPost, cfg.BaseURL+"/auth/register", "", map[string]any{
			"name":     "To Be Deleted",
			"email":    "tobedeleted@test.com",
			"password": "deleteme123",
		})
		assertStatus(t, reg.StatusCode, http.StatusCreated, reg)
		deletedID := extractNestedString(reg, "user", "id")

		do(t, client, http.MethodDelete, fmt.Sprintf("%s/users/%s", cfg.BaseURL, deletedID), adminToken, nil)

		r := do(t, client, http.MethodGet, fmt.Sprintf("%s/users/%s", cfg.BaseURL, deletedID), adminToken, nil)
		assertStatus(t, r.StatusCode, http.StatusNotFound, r)
	})

	_ = userToken     // used above
	_ = extractString // used in helpers
}
