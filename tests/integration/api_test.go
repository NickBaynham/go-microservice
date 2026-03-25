//go:build integration

// Package integration contains end-to-end tests for the go-microservice API.
//
// Tests run against a live server (local or Docker). Configure via environment:
//
//	TEST_HOST              default: localhost
//	TEST_PORT              default: 8443
//	TEST_SCHEME            default: https
//	TEST_SKIP_TLS_VERIFY   default: true  (set false for production certs)
//	MONGO_URI              MongoDB URI (must match the server under test)
//	MONGO_DB               default: userservice (same as cmd/server; must match the running server)
//	JWT_SECRET             Must match the server (default change-me-in-production; Docker test uses test-jwt-secret-do-not-use-in-prod)
//	TEST_HTTP_BASE_URL     Optional e.g. http://localhost:8080 — runs an extra /health check over plain HTTP
//	TEST_SKIP_RATE_LIMIT   If set (any value), skip the rate-limit subtest at the end of the suite
//	TEST_SERVER_WAIT_SEC   How long to poll /health before failing (default 30)
//	TEST_SKIP_FULL_DB_RESET  If set, do not delete all users in MONGO_DB before tests (only use if you know the DB is already empty; otherwise first-user admin tests fail)
//
// CI against a deployed ECS stack: use TestAPISmoke (go test -run TestAPISmoke). The runner cannot reach MongoDB in the task; full TestAPI needs DB access from the test process (e.g. docker-compose integration job).
//
// Local: start the API with the same MONGO_URI/MONGO_DB/JWT_SECRET as the test env, e.g.
//
//	JWT_SECRET=change-me-in-production make run
//	(use MONGO_DB=userservice_test for both server and tests if you want a separate DB from dev data)
//	make test-integration-local
//
//	make test-integration       (spins up isolated Docker environment)
package integration

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"go-microservice/internal/auth"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ── Test-run unique emails ────────────────────────────────────────────────────
// Using a timestamp suffix means tests never collide with leftover data from
// previous runs, even if DB cleanup fails or is skipped.

var runID = fmt.Sprintf("%d", time.Now().UnixMilli())

func email(name string) string {
	return fmt.Sprintf("%s+%s@test.com", name, runID)
}

// ── DB cleanup ────────────────────────────────────────────────────────────────

func cleanDB(t *testing.T) {
	t.Helper()
	mongoURI := getEnv("MONGO_URI", "mongodb://localhost:27017")
	mongoDB := getEnv("MONGO_DB", "userservice")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		t.Logf("cleanDB: could not connect to %s: %v (skipping cleanup)", mongoURI, err)
		return
	}
	defer client.Disconnect(ctx) //nolint:errcheck

	if err := client.Ping(ctx, nil); err != nil {
		t.Logf("cleanDB: could not ping %s: %v (skipping cleanup)", mongoURI, err)
		return
	}

	// Delete only the test run's users by email suffix to avoid wiping unrelated data
	res, err := client.Database(mongoDB).Collection("users").DeleteMany(ctx,
		bson.M{"email": bson.M{"$regex": "\\+" + runID + "@test\\.com$"}},
	)
	if err != nil {
		t.Logf("cleanDB: delete failed: %v", err)
		return
	}
	t.Logf("✔ database cleaned (deleted %d test users for run %s)", res.DeletedCount, runID)
}

// resetUsersCollection deletes every document in the users collection so the first
// POST /auth/register in this run sees Count()==0 and receives role admin. Local
// MongoDB persists between runs; without this, leftover users break those tests.
func resetUsersCollection(t *testing.T) {
	t.Helper()
	if getEnv("TEST_SKIP_FULL_DB_RESET", "") != "" {
		t.Log("TEST_SKIP_FULL_DB_RESET set — skipping full users collection reset")
		return
	}

	mongoURI := getEnv("MONGO_URI", "mongodb://localhost:27017")
	mongoDB := getEnv("MONGO_DB", "userservice")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		t.Fatalf("reset users: connect %s: %v", mongoURI, err)
	}
	defer client.Disconnect(ctx) //nolint:errcheck

	if err := client.Ping(ctx, nil); err != nil {
		t.Fatalf("reset users: ping %s: %v", mongoURI, err)
	}

	res, err := client.Database(mongoDB).Collection("users").DeleteMany(ctx, bson.M{})
	if err != nil {
		t.Fatalf("reset users: %v", err)
	}
	t.Logf("✔ cleared users collection in %q (%d documents removed)", mongoDB, res.DeletedCount)
}

// ── Wait for server ───────────────────────────────────────────────────────────

func waitForServer(t *testing.T, cfg testConfig, client *http.Client) {
	t.Helper()
	url := cfg.BaseURL + "/health"
	waitSec := 30
	if s := getEnv("TEST_SERVER_WAIT_SEC", ""); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			waitSec = v
		}
	}
	deadline := time.Now().Add(time.Duration(waitSec) * time.Second)
	var lastErr string
	for time.Now().Before(deadline) {
		resp, err := client.Get(url) //nolint:noctx
		if err != nil {
			lastErr = err.Error()
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			t.Log("✔ server is ready")
			return
		}
		lastErr = fmt.Sprintf("HTTP %d (want 200; 503 usually means MongoDB unreachable)", resp.StatusCode)
		resp.Body.Close()
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("server did not become ready within %ds calling %s\n  last: %s\n  hint: run the API in another terminal (e.g. make run for https://localhost:8443; use the same MONGO_DB as tests). If the app uses LISTEN_HTTP or ENV=production without TLS certs, it listens on plain HTTP (often :8080): TEST_SCHEME=http TEST_PORT=8080 make test-integration-local", waitSec, url, lastErr)
}

func assertHealthReturnsOK(t *testing.T, cfg testConfig, client *http.Client) {
	t.Helper()
	r := do(t, client, http.MethodGet, cfg.BaseURL+"/health", "", nil)

	assertStatus(t, r.StatusCode, http.StatusOK, r)
	assertStringField(t, r, "status", "healthy")
	assertKey(t, r, "timestamp")

	checks, ok := r.Body["checks"].(map[string]any)
	if !ok {
		t.Errorf("checks: expected object, got %T", r.Body["checks"])
		return
	}
	if checks["mongodb"] != "ok" {
		t.Errorf("checks.mongodb: got %v, want \"ok\"", checks["mongodb"])
	}
}

// TestAPISmoke hits /health over TEST_HOST (e.g. ALB DNS in GitHub Actions). It does not connect to MongoDB from the test process — required for ECS where Mongo is only reachable inside the task.
func TestAPISmoke(t *testing.T) {
	cfg := loadConfig()
	client := newClient()
	waitForServer(t, cfg, client)
	assertHealthReturnsOK(t, cfg, client)
}

// ── Main test suite ───────────────────────────────────────────────────────────

func TestAPI(t *testing.T) {
	cfg := loadConfig()
	client := newClient()

	waitForServer(t, cfg, client)
	resetUsersCollection(t)
	t.Cleanup(func() { cleanDB(t) })

	// Use unique emails for this test run
	adminEmail := email("admin")
	userEmail := email("user")
	adminName := "Test Admin"
	userName := "Test User"
	adminPass := "adminpass123"
	userPass := "userpass123"

	var (
		adminToken string
		userToken  string
		adminID    string
		userID     string
	)

	// ── Health ────────────────────────────────────────────────────────────────

	t.Run("GET /health returns healthy", func(t *testing.T) {
		assertHealthReturnsOK(t, cfg, client)
	})

	t.Run("GET /health plain HTTP optional", func(t *testing.T) {
		base := getEnv("TEST_HTTP_BASE_URL", "")
		if base == "" {
			t.Skip("set TEST_HTTP_BASE_URL to exercise HTTP-only mode, e.g. http://localhost:8080")
		}
		cl := &http.Client{Timeout: 10 * time.Second}
		resp, err := cl.Get(base + "/health") //nolint:noctx
		if err != nil {
			t.Fatalf("GET %s/health: %v", base, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status: got %d, want 200", resp.StatusCode)
		}
	})

	// ── Registration ─────────────────────────────────────────────────────────

	t.Run("POST /auth/register creates admin user", func(t *testing.T) {
		r := do(t, client, http.MethodPost, cfg.BaseURL+"/auth/register", "", map[string]any{
			"name":     adminName,
			"email":    adminEmail,
			"password": adminPass,
		})

		assertStatus(t, r.StatusCode, http.StatusCreated, r)
		assertStringField(t, r, "message", "user registered successfully")
		assertNestedStringField(t, r, "user", "name", adminName)
		assertNestedStringField(t, r, "user", "email", adminEmail)
		assertNestedStringField(t, r, "user", "role", "admin")

		if user, ok := r.Body["user"].(map[string]any); ok {
			if _, hasPassword := user["password"]; hasPassword {
				t.Error("password field must not be returned in response")
			}
		}

		adminID = extractNestedString(r, "user", "id")
		if adminID == "" {
			t.Fatal("expected non-empty user id in response")
		}
	})

	t.Run("GET /me rejects expired JWT", func(t *testing.T) {
		if adminID == "" {
			t.Skip("adminID not set")
		}
		secret := getEnv("JWT_SECRET", "change-me-in-production")
		tok, err := auth.GenerateToken(adminID, adminEmail, "admin", secret, "0")
		if err != nil {
			t.Fatalf("GenerateToken: %v", err)
		}
		time.Sleep(1300 * time.Millisecond)
		r := do(t, client, http.MethodGet, cfg.BaseURL+"/me", tok, nil)
		assertStatus(t, r.StatusCode, http.StatusUnauthorized, r)
		assertKey(t, r, "error")
	})

	t.Run("POST /auth/register creates regular user", func(t *testing.T) {
		r := do(t, client, http.MethodPost, cfg.BaseURL+"/auth/register", "", map[string]any{
			"name":     userName,
			"email":    userEmail,
			"password": userPass,
		})

		assertStatus(t, r.StatusCode, http.StatusCreated, r)
		assertNestedStringField(t, r, "user", "role", "user")

		userID = extractNestedString(r, "user", "id")
		if userID == "" {
			t.Fatal("expected non-empty user id in response")
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
			"email":    email("shortpass"),
			"password": "123",
		})
		assertStatus(t, r.StatusCode, http.StatusBadRequest, r)
		assertKey(t, r, "error")
	})

	t.Run("POST /auth/register rejects invalid email", func(t *testing.T) {
		r := do(t, client, http.MethodPost, cfg.BaseURL+"/auth/register", "", map[string]any{
			"name":     "Bad Email",
			"email":    "not-an-email",
			"password": "password12",
		})
		assertStatus(t, r.StatusCode, http.StatusBadRequest, r)
		assertKey(t, r, "error")
	})

	t.Run("POST /auth/register rejects short name", func(t *testing.T) {
		r := do(t, client, http.MethodPost, cfg.BaseURL+"/auth/register", "", map[string]any{
			"name":     "A",
			"email":    email("shortname"),
			"password": "password12",
		})
		assertStatus(t, r.StatusCode, http.StatusBadRequest, r)
		assertKey(t, r, "error")
	})

	t.Run("POST /auth/register rejects malformed JSON", func(t *testing.T) {
		r := doRaw(t, client, http.MethodPost, cfg.BaseURL+"/auth/register", "", "application/json", `{"name":`)
		if r.StatusCode != http.StatusBadRequest {
			t.Errorf("status: got %d, want 400\nbody: %s", r.StatusCode, r.RawBody)
		}
	})

	t.Run("POST /auth/register concurrent same email one succeeds", func(t *testing.T) {
		em := email("concurrent")
		payload := map[string]any{
			"name":     "Concurrent",
			"email":    em,
			"password": "concurrent12",
		}
		ch := make(chan int, 2)
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, err := doRequest(client, http.MethodPost, cfg.BaseURL+"/auth/register", "", payload)
				if err != nil {
					ch <- -1
					return
				}
				ch <- r.StatusCode
			}()
		}
		wg.Wait()
		c1, c2 := <-ch, <-ch
		for _, c := range []int{c1, c2} {
			if c == -1 {
				t.Fatal("concurrent register request failed")
			}
		}
		codes := []int{c1, c2}
		sort.Ints(codes)
		if len(codes) != 2 || codes[0] != http.StatusCreated || codes[1] != http.StatusConflict {
			t.Errorf("status codes: got %v, want [201 409] in any order", []int{c1, c2})
		}
	})

	// ── Login ─────────────────────────────────────────────────────────────────

	t.Run("POST /auth/login returns token for admin", func(t *testing.T) {
		r := do(t, client, http.MethodPost, cfg.BaseURL+"/auth/login", "", map[string]any{
			"email":    adminEmail,
			"password": adminPass,
		})

		assertStatus(t, r.StatusCode, http.StatusOK, r)
		adminToken = assertNonEmptyString(t, r, "token")
		assertNestedStringField(t, r, "user", "email", adminEmail)
		assertNestedStringField(t, r, "user", "role", "admin")

		if user, ok := r.Body["user"].(map[string]any); ok {
			if _, hasPassword := user["password"]; hasPassword {
				t.Error("password field must not be returned in login response")
			}
		}
	})

	t.Run("POST /auth/login returns token for user", func(t *testing.T) {
		r := do(t, client, http.MethodPost, cfg.BaseURL+"/auth/login", "", map[string]any{
			"email":    userEmail,
			"password": userPass,
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

	t.Run("POST /auth/login rejects missing password", func(t *testing.T) {
		r := do(t, client, http.MethodPost, cfg.BaseURL+"/auth/login", "", map[string]any{
			"email": adminEmail,
		})
		assertStatus(t, r.StatusCode, http.StatusBadRequest, r)
		assertKey(t, r, "error")
	})

	// ── GET /me ───────────────────────────────────────────────────────────────

	t.Run("GET /me returns current user", func(t *testing.T) {
		if adminToken == "" {
			t.Skip("adminToken not set — login test failed")
		}
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

	// ── GET /users ────────────────────────────────────────────────────────────

	t.Run("GET /users returns all users for admin", func(t *testing.T) {
		if adminToken == "" {
			t.Skip("adminToken not set — login test failed")
		}
		r := do(t, client, http.MethodGet, cfg.BaseURL+"/users", adminToken, nil)
		assertStatus(t, r.StatusCode, http.StatusOK, r)
		assertKey(t, r, "users")
		assertKey(t, r, "count")

		rawUsers, ok := r.Body["users"].([]any)
		if !ok {
			t.Fatalf("users: expected array, got %T", r.Body["users"])
		}
		for i, item := range rawUsers {
			u, ok := item.(map[string]any)
			if !ok {
				t.Fatalf("users[%d]: expected object, got %T", i, item)
			}
			if _, has := u["password"]; has {
				t.Errorf("users[%d]: password must not appear in list response", i)
			}
		}
	})

	t.Run("GET /users rejects regular user", func(t *testing.T) {
		if userToken == "" {
			t.Skip("userToken not set — login test failed")
		}
		r := do(t, client, http.MethodGet, cfg.BaseURL+"/users", userToken, nil)
		assertStatus(t, r.StatusCode, http.StatusForbidden, r)
		assertKey(t, r, "error")
	})

	t.Run("GET /users rejects unauthenticated request", func(t *testing.T) {
		r := do(t, client, http.MethodGet, cfg.BaseURL+"/users", "", nil)
		assertStatus(t, r.StatusCode, http.StatusUnauthorized, r)
		assertKey(t, r, "error")
	})

	// ── GET /users/:id ────────────────────────────────────────────────────────

	t.Run("GET /users/:id returns user for admin", func(t *testing.T) {
		if adminToken == "" || userID == "" {
			t.Skip("adminToken or userID not set — earlier test failed")
		}
		r := do(t, client, http.MethodGet, fmt.Sprintf("%s/users/%s", cfg.BaseURL, userID), adminToken, nil)
		assertStatus(t, r.StatusCode, http.StatusOK, r)
		assertStringField(t, r, "email", userEmail)
		assertStringField(t, r, "name", userName)
		assertStringField(t, r, "role", "user")
	})

	t.Run("GET /users/:id returns 404 for unknown id", func(t *testing.T) {
		if adminToken == "" {
			t.Skip("adminToken not set — login test failed")
		}
		r := do(t, client, http.MethodGet, cfg.BaseURL+"/users/000000000000000000000000", adminToken, nil)
		assertStatus(t, r.StatusCode, http.StatusNotFound, r)
		assertKey(t, r, "error")
	})

	t.Run("GET /users/:id rejects regular user", func(t *testing.T) {
		if userToken == "" || adminID == "" {
			t.Skip("userToken or adminID not set — earlier test failed")
		}
		r := do(t, client, http.MethodGet, fmt.Sprintf("%s/users/%s", cfg.BaseURL, adminID), userToken, nil)
		assertStatus(t, r.StatusCode, http.StatusForbidden, r)
		assertKey(t, r, "error")
	})

	t.Run("GET /users/:id rejects invalid id", func(t *testing.T) {
		if adminToken == "" {
			t.Skip("adminToken not set — login test failed")
		}
		r := do(t, client, http.MethodGet, cfg.BaseURL+"/users/not-a-valid-object-id", adminToken, nil)
		assertStatus(t, r.StatusCode, http.StatusNotFound, r)
		assertKey(t, r, "error")
	})

	// ── PUT /users/:id ────────────────────────────────────────────────────────

	t.Run("PUT /users/:id user can update own profile", func(t *testing.T) {
		if userToken == "" || userID == "" {
			t.Skip("userToken or userID not set — earlier test failed")
		}
		r := do(t, client, http.MethodPut, fmt.Sprintf("%s/users/%s", cfg.BaseURL, userID), userToken, map[string]any{
			"name": "Updated User Name",
		})
		assertStatus(t, r.StatusCode, http.StatusOK, r)
		assertStringField(t, r, "name", "Updated User Name")
		assertStringField(t, r, "email", userEmail)
	})

	t.Run("PUT /users/:id user cannot update another user", func(t *testing.T) {
		if userToken == "" || adminID == "" {
			t.Skip("userToken or adminID not set — earlier test failed")
		}
		r := do(t, client, http.MethodPut, fmt.Sprintf("%s/users/%s", cfg.BaseURL, adminID), userToken, map[string]any{
			"name": "Hacked Name",
		})
		assertStatus(t, r.StatusCode, http.StatusForbidden, r)
		assertKey(t, r, "error")
	})

	t.Run("PUT /users/:id admin can update any user", func(t *testing.T) {
		if adminToken == "" || userID == "" {
			t.Skip("adminToken or userID not set — earlier test failed")
		}
		r := do(t, client, http.MethodPut, fmt.Sprintf("%s/users/%s", cfg.BaseURL, userID), adminToken, map[string]any{
			"name": "Admin Updated Name",
		})
		assertStatus(t, r.StatusCode, http.StatusOK, r)
		assertStringField(t, r, "name", "Admin Updated Name")
	})

	t.Run("PUT /users/:id admin can change user role", func(t *testing.T) {
		if adminToken == "" || userID == "" {
			t.Skip("adminToken or userID not set — earlier test failed")
		}
		r := do(t, client, http.MethodPut, fmt.Sprintf("%s/users/%s", cfg.BaseURL, userID), adminToken, map[string]any{
			"role": "admin",
		})
		assertStatus(t, r.StatusCode, http.StatusOK, r)
		assertStringField(t, r, "role", "admin")

		// Reset role back
		do(t, client, http.MethodPut, fmt.Sprintf("%s/users/%s", cfg.BaseURL, userID), adminToken, map[string]any{
			"role": "user",
		})
	})

	t.Run("PUT /users/:id rejects empty update body", func(t *testing.T) {
		if adminToken == "" || adminID == "" {
			t.Skip("adminToken or adminID not set — earlier test failed")
		}
		r := do(t, client, http.MethodPut, fmt.Sprintf("%s/users/%s", cfg.BaseURL, adminID), adminToken, map[string]any{})
		assertStatus(t, r.StatusCode, http.StatusBadRequest, r)
		assertKey(t, r, "error")
	})

	t.Run("PUT /users/:id rejects unauthenticated request", func(t *testing.T) {
		if userID == "" {
			t.Skip("userID not set — register test failed")
		}
		r := do(t, client, http.MethodPut, fmt.Sprintf("%s/users/%s", cfg.BaseURL, userID), "", map[string]any{
			"name": "Ghost",
		})
		assertStatus(t, r.StatusCode, http.StatusUnauthorized, r)
		assertKey(t, r, "error")
	})

	t.Run("PUT /users/:id rejects invalid id", func(t *testing.T) {
		if adminToken == "" {
			t.Skip("adminToken not set — login test failed")
		}
		r := do(t, client, http.MethodPut, cfg.BaseURL+"/users/not-a-valid-object-id", adminToken, map[string]any{
			"name": "Valid Name",
		})
		assertStatus(t, r.StatusCode, http.StatusNotFound, r)
		assertKey(t, r, "error")
	})

	t.Run("PUT /users/:id returns 409 when email collides", func(t *testing.T) {
		if adminToken == "" {
			t.Skip("adminToken not set — login test failed")
		}
		emailA := email("collisionA")
		emailB := email("collisionB")
		regA := do(t, client, http.MethodPost, cfg.BaseURL+"/auth/register", "", map[string]any{
			"name": "Collision A", "email": emailA, "password": "collisionpass1",
		})
		assertStatus(t, regA.StatusCode, http.StatusCreated, regA)
		regB := do(t, client, http.MethodPost, cfg.BaseURL+"/auth/register", "", map[string]any{
			"name": "Collision B", "email": emailB, "password": "collisionpass2",
		})
		assertStatus(t, regB.StatusCode, http.StatusCreated, regB)
		idB := extractNestedString(regB, "user", "id")
		if idB == "" {
			t.Fatal("missing user id for collision B")
		}
		r := do(t, client, http.MethodPut, fmt.Sprintf("%s/users/%s", cfg.BaseURL, idB), adminToken, map[string]any{
			"email": emailA,
		})
		assertStatus(t, r.StatusCode, http.StatusConflict, r)
		assertKey(t, r, "error")
	})

	// ── DELETE /users/:id ─────────────────────────────────────────────────────

	t.Run("DELETE /users/:id rejects regular user", func(t *testing.T) {
		if userToken == "" || adminID == "" {
			t.Skip("userToken or adminID not set — earlier test failed")
		}
		r := do(t, client, http.MethodDelete, fmt.Sprintf("%s/users/%s", cfg.BaseURL, adminID), userToken, nil)
		assertStatus(t, r.StatusCode, http.StatusForbidden, r)
		assertKey(t, r, "error")
	})

	t.Run("DELETE /users/:id rejects unauthenticated request", func(t *testing.T) {
		if userID == "" {
			t.Skip("userID not set — register test failed")
		}
		r := do(t, client, http.MethodDelete, fmt.Sprintf("%s/users/%s", cfg.BaseURL, userID), "", nil)
		assertStatus(t, r.StatusCode, http.StatusUnauthorized, r)
		assertKey(t, r, "error")
	})

	t.Run("DELETE /users/:id admin can delete user", func(t *testing.T) {
		if adminToken == "" {
			t.Skip("adminToken not set — login test failed")
		}
		reg := do(t, client, http.MethodPost, cfg.BaseURL+"/auth/register", "", map[string]any{
			"name":     "Throwaway User",
			"email":    email("throwaway"),
			"password": "throwaway123",
		})
		assertStatus(t, reg.StatusCode, http.StatusCreated, reg)
		throwawayID := extractNestedString(reg, "user", "id")

		r := do(t, client, http.MethodDelete, fmt.Sprintf("%s/users/%s", cfg.BaseURL, throwawayID), adminToken, nil)
		assertStatus(t, r.StatusCode, http.StatusOK, r)
		assertStringField(t, r, "message", "user deleted successfully")
	})

	t.Run("DELETE /users/:id returns 404 for unknown id", func(t *testing.T) {
		if adminToken == "" {
			t.Skip("adminToken not set — login test failed")
		}
		r := do(t, client, http.MethodDelete, cfg.BaseURL+"/users/000000000000000000000000", adminToken, nil)
		assertStatus(t, r.StatusCode, http.StatusNotFound, r)
		assertKey(t, r, "error")
	})

	t.Run("DELETE /users/:id rejects invalid id", func(t *testing.T) {
		if adminToken == "" {
			t.Skip("adminToken not set — login test failed")
		}
		r := do(t, client, http.MethodDelete, cfg.BaseURL+"/users/not-a-valid-object-id", adminToken, nil)
		assertStatus(t, r.StatusCode, http.StatusNotFound, r)
		assertKey(t, r, "error")
	})

	t.Run("GET /users/:id returns 404 after deletion", func(t *testing.T) {
		if adminToken == "" {
			t.Skip("adminToken not set — login test failed")
		}
		reg := do(t, client, http.MethodPost, cfg.BaseURL+"/auth/register", "", map[string]any{
			"name":     "To Be Deleted",
			"email":    email("tobedeleted"),
			"password": "deleteme123",
		})
		assertStatus(t, reg.StatusCode, http.StatusCreated, reg)
		deletedID := extractNestedString(reg, "user", "id")

		do(t, client, http.MethodDelete, fmt.Sprintf("%s/users/%s", cfg.BaseURL, deletedID), adminToken, nil)

		r := do(t, client, http.MethodGet, fmt.Sprintf("%s/users/%s", cfg.BaseURL, deletedID), adminToken, nil)
		assertStatus(t, r.StatusCode, http.StatusNotFound, r)
	})

	// Run last: flooding /auth/login exhausts the per-IP bucket. With a single shared limiter
	// (older builds) or a short refill window, doing this at the start caused 429 on later /auth/register calls.
	t.Run("POST /auth/login returns 429 when rate limited", func(t *testing.T) {
		if getEnv("TEST_SKIP_RATE_LIMIT", "") != "" {
			t.Skip("TEST_SKIP_RATE_LIMIT is set")
		}
		n429 := 0
		for i := 0; i < 40; i++ {
			r := do(t, client, http.MethodPost, cfg.BaseURL+"/auth/login", "", map[string]any{
				"email":    "rate-limit-does-not-exist@test.com",
				"password": "wrong-password-for-rate-limit",
			})
			if r.StatusCode == http.StatusTooManyRequests {
				n429++
			}
		}
		if n429 == 0 {
			t.Error("expected at least one 429 Too Many Requests from rate limiter")
		}
	})
}
