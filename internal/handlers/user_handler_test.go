package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go-microservice/internal/auth"
	"go-microservice/internal/config"
	"go-microservice/internal/handlers"
	"go-microservice/internal/models"
	"go-microservice/internal/repository"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func init() {
	gin.SetMode(gin.TestMode)
}

const testJWTSecret = "unit-test-jwt-secret-must-be-32chars!"

func testConfig() *config.Config {
	return &config.Config{
		JWTSecret:                 testJWTSecret,
		JWTExpireHours:            "24",
		JWTAccessExpireMinutes:    1440,
		JWTRefreshExpireHours:    720,
		Env:                       "development",
		PasswordResetFrontendURL:  "http://localhost:5173/reset-password",
		PasswordResetTokenMinutes: 60,
		EmailChangeFrontendURL:    "http://localhost:5173/confirm-email-change",
		EmailChangeTokenMinutes:   60,
	}
}

type refreshEntry struct {
	userID    primitive.ObjectID
	familyID  string
	expiresAt time.Time
}

// mapRefreshStore is an in-memory refresh session store for handler tests.
type mapRefreshStore struct {
	mu     sync.Mutex
	byHash map[string]refreshEntry
}

func newMapRefreshStore() *mapRefreshStore {
	return &mapRefreshStore{byHash: make(map[string]refreshEntry)}
}

func (m *mapRefreshStore) Insert(_ context.Context, userID primitive.ObjectID, tokenHash, familyID string, expiresAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byHash[tokenHash] = refreshEntry{userID: userID, familyID: familyID, expiresAt: expiresAt}
	return nil
}

func (m *mapRefreshStore) ConsumeAndRotate(_ context.Context, presentedHash, newHash string, newExpires time.Time) (*repository.RefreshTokenDoc, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	old, ok := m.byHash[presentedHash]
	if !ok || !time.Now().Before(old.expiresAt) {
		return nil, repository.ErrInvalidRefreshToken
	}
	if presentedHash == newHash {
		m.byHash[presentedHash] = refreshEntry{userID: old.userID, familyID: old.familyID, expiresAt: newExpires}
	} else {
		if _, taken := m.byHash[newHash]; taken {
			return nil, errors.New("refresh token hash collision")
		}
		m.byHash[newHash] = refreshEntry{userID: old.userID, familyID: old.familyID, expiresAt: newExpires}
		delete(m.byHash, presentedHash)
	}
	return &repository.RefreshTokenDoc{UserID: old.userID, FamilyID: old.familyID, TokenHash: presentedHash}, nil
}

func (m *mapRefreshStore) DeleteByHash(_ context.Context, tokenHash string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.byHash[tokenHash]
	if !ok || !time.Now().Before(e.expiresAt) {
		return false, nil
	}
	delete(m.byHash, tokenHash)
	return true, nil
}

func (m *mapRefreshStore) DeleteAllForUser(_ context.Context, userID primitive.ObjectID) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for h, e := range m.byHash {
		if e.userID == userID {
			delete(m.byHash, h)
			n++
		}
	}
	return n, nil
}

type mockUserStore struct {
	count      int64
	countErr   error
	findAllErr error
	users      []models.User
	byEmail    map[string]int // email -> index in users
}

func newMockStore() *mockUserStore {
	return &mockUserStore{byEmail: make(map[string]int)}
}

func (m *mockUserStore) userIndexByID(id primitive.ObjectID) int {
	for i := range m.users {
		if m.users[i].ID == id {
			return i
		}
	}
	return -1
}

func (m *mockUserStore) Count(ctx context.Context) (int64, error) {
	if m.countErr != nil {
		return 0, m.countErr
	}
	return m.count, nil
}

func (m *mockUserStore) Create(ctx context.Context, user *models.User) error {
	if _, exists := m.byEmail[user.Email]; exists {
		return repository.ErrDuplicateEmail
	}
	if user.ID.IsZero() {
		user.ID = primitive.NewObjectID()
	}
	m.users = append(m.users, *user)
	m.byEmail[user.Email] = len(m.users) - 1
	m.count = int64(len(m.users))
	return nil
}

func (m *mockUserStore) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	i, ok := m.byEmail[email]
	if !ok {
		return nil, errNotFound
	}
	u := m.users[i]
	return &u, nil
}

func (m *mockUserStore) FindAll(ctx context.Context) ([]models.User, error) {
	if m.findAllErr != nil {
		return nil, m.findAllErr
	}
	return m.users, nil
}

func (m *mockUserStore) FindByID(ctx context.Context, id string) (*models.User, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, errNotFound
	}
	for i := range m.users {
		if m.users[i].ID == oid {
			u := m.users[i]
			return &u, nil
		}
	}
	return nil, errNotFound
}

func (m *mockUserStore) Update(ctx context.Context, id string, update bson.M) (*models.User, error) {
	u, err := m.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	idx := m.userIndexByID(u.ID)
	if idx < 0 {
		return nil, errNotFound
	}

	if v, ok := update["pending_email"].(string); ok {
		u.PendingEmail = v
	}
	newEmail := u.Email
	if v, ok := update["email"].(string); ok {
		newEmail = v
		if newEmail != u.Email {
			if existingIdx, exists := m.byEmail[newEmail]; exists && m.users[existingIdx].ID != u.ID {
				return nil, repository.ErrDuplicateEmail
			}
			delete(m.byEmail, u.Email)
			m.byEmail[newEmail] = idx
		}
	}
	if v, ok := update["name"].(string); ok {
		u.Name = v
	}
	u.Email = newEmail
	if v, ok := update["role"].(string); ok {
		u.Role = v
	}
	if v, ok := update["password"].(string); ok {
		u.Password = v
	}
	if v, ok := update["email_verified"].(bool); ok {
		u.EmailVerified = v
	}
	switch v := update["failed_login_attempts"].(type) {
	case int:
		u.FailedLoginAttempts = v
	case int32:
		u.FailedLoginAttempts = int(v)
	case int64:
		u.FailedLoginAttempts = int(v)
	}
	if v, ok := update["locked_until"].(*time.Time); ok {
		u.LockedUntil = v
	}
	m.users[idx] = *u
	return u, nil
}

func (m *mockUserStore) IncrementFailedLogin(_ context.Context, userID string, maxAttempts int, lockout time.Duration) error {
	u, err := m.FindByID(context.Background(), userID)
	if err != nil {
		return err
	}
	idx := m.userIndexByID(u.ID)
	if idx < 0 {
		return errNotFound
	}
	u.FailedLoginAttempts++
	if maxAttempts > 0 && u.FailedLoginAttempts >= maxAttempts {
		t := time.Now().Add(lockout)
		u.LockedUntil = &t
	}
	m.users[idx] = *u
	return nil
}

func (m *mockUserStore) ClearLoginLockout(_ context.Context, userID string) error {
	u, err := m.FindByID(context.Background(), userID)
	if err != nil {
		return err
	}
	idx := m.userIndexByID(u.ID)
	if idx < 0 {
		return errNotFound
	}
	u.FailedLoginAttempts = 0
	u.LockedUntil = nil
	m.users[idx] = *u
	return nil
}

func (m *mockUserStore) Delete(ctx context.Context, id string) error {
	u, err := m.FindByID(ctx, id)
	if err != nil {
		return err
	}
	delete(m.byEmail, u.Email)
	next := make([]models.User, 0, len(m.users)-1)
	m.byEmail = make(map[string]int)
	for _, x := range m.users {
		if x.ID != u.ID {
			next = append(next, x)
			m.byEmail[x.Email] = len(next) - 1
		}
	}
	m.users = next
	m.count = int64(len(m.users))
	return nil
}

var errNotFound = errors.New("not found")

func registerUser(t *testing.T, h *handlers.UserHandler, name, email, password string) *models.User {
	t.Helper()
	body := `{"name":"` + name + `","email":"` + email + `","password":"` + password + `"}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader([]byte(body)))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Register(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", w.Code, w.Body.String())
	}
	var resp models.RegisterResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return &resp.User
}

func TestRegister_FirstUserBecomesAdmin(t *testing.T) {
	store := newMockStore()
	h := handlers.NewUserHandler(store, newMapRefreshStore(), testConfig(), nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := `{"name":"Alice","email":"a@example.com","password":"password12"}`
	c.Request = httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader([]byte(body)))
	c.Request.Header.Set("Content-Type", "application/json")

	h.Register(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("status %d, body %s", w.Code, w.Body.String())
	}
	var resp models.RegisterResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.User.Role != "admin" {
		t.Errorf("role: got %q, want admin", resp.User.Role)
	}
}

func TestRegister_SecondUserIsUser(t *testing.T) {
	store := newMockStore()
	h := handlers.NewUserHandler(store, newMapRefreshStore(), testConfig(), nil)

	first := `{"name":"Admin","email":"admin@example.com","password":"password12"}`
	w1 := httptest.NewRecorder()
	c1, _ := gin.CreateTestContext(w1)
	c1.Request = httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader([]byte(first)))
	c1.Request.Header.Set("Content-Type", "application/json")
	h.Register(c1)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first register: %d %s", w1.Code, w1.Body.String())
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	second := `{"name":"Bob","email":"b@example.com","password":"password12"}`
	c.Request = httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader([]byte(second)))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Register(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("status %d, body %s", w.Code, w.Body.String())
	}
	var resp models.RegisterResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.User.Role != "user" {
		t.Errorf("role: got %q, want user", resp.User.Role)
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	store := newMockStore()
	h := handlers.NewUserHandler(store, newMapRefreshStore(), testConfig(), nil)
	payload := []byte(`{"name":"Alice","email":"dup@example.com","password":"password12"}`)

	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(payload))
		c.Request.Header.Set("Content-Type", "application/json")
		h.Register(c)
		if i == 0 && w.Code != http.StatusCreated {
			t.Fatalf("first: %d %s", w.Code, w.Body.String())
		}
		if i == 1 && w.Code != http.StatusConflict {
			t.Fatalf("second: want 409, got %d %s", w.Code, w.Body.String())
		}
	}
}

func TestRegister_CountError(t *testing.T) {
	store := newMockStore()
	store.countErr = errors.New("database unavailable")
	h := handlers.NewUserHandler(store, newMapRefreshStore(), testConfig(), nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader([]byte(
		`{"name":"XX","email":"x@example.com","password":"password12"}`,
	)))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Register(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d, want 500", w.Code)
	}
}

func TestLogin_Success(t *testing.T) {
	store := newMockStore()
	h := handlers.NewUserHandler(store, newMapRefreshStore(), testConfig(), nil)
	u := registerUser(t, h, "Alice", "alice@example.com", "secretpass99")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader([]byte(
		`{"email":"alice@example.com","password":"secretpass99"}`,
	)))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Login(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var resp models.LoginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Token == "" || resp.AccessToken == "" {
		t.Error("expected token and access_token")
	}
	if resp.Token != resp.AccessToken {
		t.Errorf("token should mirror access_token")
	}
	if resp.RefreshToken == "" {
		t.Error("expected refresh_token")
	}
	if resp.ExpiresIn != 1440*60 {
		t.Errorf("expires_in: got %d want %d", resp.ExpiresIn, 1440*60)
	}
	if resp.User.ID != u.ID {
		t.Errorf("user id mismatch")
	}
}

func TestRefresh_Success(t *testing.T) {
	store := newMockStore()
	h := handlers.NewUserHandler(store, newMapRefreshStore(), testConfig(), nil)
	registerUser(t, h, "Alice", "alice@example.com", "secretpass99")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader([]byte(
		`{"email":"alice@example.com","password":"secretpass99"}`,
	)))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Login(c)
	if w.Code != http.StatusOK {
		t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}
	var login models.LoginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &login); err != nil {
		t.Fatal(err)
	}

	body := `{"refresh_token":"` + login.RefreshToken + `"}`
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/auth/refresh", bytes.NewReader([]byte(body)))
	c2.Request.Header.Set("Content-Type", "application/json")
	h.Refresh(c2)
	if w2.Code != http.StatusOK {
		t.Fatalf("refresh: %d %s", w2.Code, w2.Body.String())
	}
	var ref models.RefreshResponse
	if err := json.Unmarshal(w2.Body.Bytes(), &ref); err != nil {
		t.Fatal(err)
	}
	if ref.AccessToken == "" || ref.RefreshToken == "" || ref.Token != ref.AccessToken {
		t.Fatalf("refresh response: %+v", ref)
	}
	if ref.RefreshToken == login.RefreshToken {
		t.Error("refresh token should rotate")
	}

	w3 := httptest.NewRecorder()
	c3, _ := gin.CreateTestContext(w3)
	c3.Request = httptest.NewRequest(http.MethodPost, "/auth/refresh", bytes.NewReader([]byte(
		`{"refresh_token":"`+login.RefreshToken+`"}`,
	)))
	c3.Request.Header.Set("Content-Type", "application/json")
	h.Refresh(c3)
	if w3.Code != http.StatusUnauthorized {
		t.Errorf("reuse old refresh: got %d want 401", w3.Code)
	}
}

func TestRefresh_InvalidToken(t *testing.T) {
	store := newMockStore()
	h := handlers.NewUserHandler(store, newMapRefreshStore(), testConfig(), nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/auth/refresh", bytes.NewReader([]byte(
		`{"refresh_token":"deadbeef"}`,
	)))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Refresh(c)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", w.Code)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	store := newMockStore()
	h := handlers.NewUserHandler(store, newMapRefreshStore(), testConfig(), nil)
	registerUser(t, h, "Alice", "alice@example.com", "rightpass")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader([]byte(
		`{"email":"alice@example.com","password":"wrongpass"}`,
	)))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Login(c)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", w.Code)
	}
}

func TestLogin_UnknownEmail(t *testing.T) {
	store := newMockStore()
	h := handlers.NewUserHandler(store, newMapRefreshStore(), testConfig(), nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader([]byte(
		`{"email":"nobody@example.com","password":"x"}`,
	)))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Login(c)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", w.Code)
	}
}

func TestLogin_TokenGenerationFailure(t *testing.T) {
	store := newMockStore()
	cfg := testConfig()
	cfg.JWTAccessExpireMinutes = -1
	h := handlers.NewUserHandler(store, newMapRefreshStore(), cfg, nil)
	registerUser(t, h, "Alice", "alice@example.com", "password12")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader([]byte(
		`{"email":"alice@example.com","password":"password12"}`,
	)))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Login(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d, want 500", w.Code)
	}
}

func TestGetMe_Success(t *testing.T) {
	store := newMockStore()
	h := handlers.NewUserHandler(store, newMapRefreshStore(), testConfig(), nil)
	u := registerUser(t, h, "Me", "me@example.com", "password12")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("userID", u.ID.Hex())
	c.Request = httptest.NewRequest(http.MethodGet, "/me", nil)
	h.GetMe(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var got models.User
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Email != "me@example.com" {
		t.Errorf("email: got %q", got.Email)
	}
}

func TestGetMe_NotFound(t *testing.T) {
	store := newMockStore()
	h := handlers.NewUserHandler(store, newMapRefreshStore(), testConfig(), nil)
	fakeID := primitive.NewObjectID().Hex()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("userID", fakeID)
	c.Request = httptest.NewRequest(http.MethodGet, "/me", nil)
	h.GetMe(c)
	if w.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", w.Code)
	}
}

func TestListUsers_Success(t *testing.T) {
	store := newMockStore()
	h := handlers.NewUserHandler(store, newMapRefreshStore(), testConfig(), nil)
	registerUser(t, h, "Alice", "a@example.com", "password12")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/users", nil)
	h.ListUsers(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var resp models.ListUsersResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Count != 1 || len(resp.Users) != 1 {
		t.Errorf("count: got %d users %d", resp.Count, len(resp.Users))
	}
}

func TestListUsers_RepositoryError(t *testing.T) {
	store := newMockStore()
	store.findAllErr = errors.New("db error")
	h := handlers.NewUserHandler(store, newMapRefreshStore(), testConfig(), nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/users", nil)
	h.ListUsers(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d, want 500", w.Code)
	}
}

func TestGetUser_Success(t *testing.T) {
	store := newMockStore()
	h := handlers.NewUserHandler(store, newMapRefreshStore(), testConfig(), nil)
	u := registerUser(t, h, "Bob", "bob@example.com", "password12")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: u.ID.Hex()}}
	c.Request = httptest.NewRequest(http.MethodGet, "/users/"+u.ID.Hex(), nil)
	h.GetUser(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
}

func TestGetUser_NotFound(t *testing.T) {
	store := newMockStore()
	h := handlers.NewUserHandler(store, newMapRefreshStore(), testConfig(), nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: primitive.NewObjectID().Hex()}}
	c.Request = httptest.NewRequest(http.MethodGet, "/users/x", nil)
	h.GetUser(c)
	if w.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", w.Code)
	}
}

func TestUpdateUser_ForbiddenWhenNotSelfAndNotAdmin(t *testing.T) {
	store := newMockStore()
	h := handlers.NewUserHandler(store, newMapRefreshStore(), testConfig(), nil)
	admin := registerUser(t, h, "Admin", "admin@example.com", "password12")
	user := registerUser(t, h, "User", "user@example.com", "password12")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("userID", user.ID.Hex())
	c.Set("role", "user")
	c.Params = gin.Params{{Key: "id", Value: admin.ID.Hex()}}
	c.Request = httptest.NewRequest(http.MethodPut, "/users/"+admin.ID.Hex(), bytes.NewReader([]byte(`{"name":"Hacker"}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	h.UpdateUser(c)
	if w.Code != http.StatusForbidden {
		t.Errorf("status: got %d, want 403", w.Code)
	}
}

func TestUpdateUser_Self(t *testing.T) {
	store := newMockStore()
	h := handlers.NewUserHandler(store, newMapRefreshStore(), testConfig(), nil)
	u := registerUser(t, h, "Old", "self@example.com", "password12")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("userID", u.ID.Hex())
	c.Set("role", "user")
	c.Params = gin.Params{{Key: "id", Value: u.ID.Hex()}}
	c.Request = httptest.NewRequest(http.MethodPut, "/users/"+u.ID.Hex(), bytes.NewReader([]byte(`{"name":"New Name"}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	h.UpdateUser(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateUser_AdminSetsRole(t *testing.T) {
	store := newMockStore()
	h := handlers.NewUserHandler(store, newMapRefreshStore(), testConfig(), nil)
	registerUser(t, h, "Admin", "admin@example.com", "password12")
	target := registerUser(t, h, "User", "user@example.com", "password12")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("userID", store.users[0].ID.Hex())
	c.Set("role", "admin")
	c.Params = gin.Params{{Key: "id", Value: target.ID.Hex()}}
	c.Request = httptest.NewRequest(http.MethodPut, "/users/"+target.ID.Hex(), bytes.NewReader([]byte(`{"role":"admin"}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	h.UpdateUser(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var got models.User
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Role != "admin" {
		t.Errorf("role: got %q", got.Role)
	}
}

func TestUpdateUser_NonAdminCannotSetRole(t *testing.T) {
	store := newMockStore()
	h := handlers.NewUserHandler(store, newMapRefreshStore(), testConfig(), nil)
	registerUser(t, h, "Admin", "admin@example.com", "password12")
	u := registerUser(t, h, "User", "user@example.com", "password12")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("userID", u.ID.Hex())
	c.Set("role", "user")
	c.Params = gin.Params{{Key: "id", Value: u.ID.Hex()}}
	c.Request = httptest.NewRequest(http.MethodPut, "/users/"+u.ID.Hex(), bytes.NewReader([]byte(`{"role":"admin"}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	h.UpdateUser(c)
	// Role is stripped for non-admins; no other fields → "no fields to update"
	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400 (no fields to update)", w.Code)
	}
}

func TestUpdateUser_DuplicateEmail(t *testing.T) {
	store := newMockStore()
	h := handlers.NewUserHandler(store, newMapRefreshStore(), testConfig(), nil)
	registerUser(t, h, "Admin", "admin@example.com", "password12")
	registerUser(t, h, "Alice", "a@example.com", "password12")
	b := registerUser(t, h, "Bob", "b@example.com", "password12")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("userID", store.users[0].ID.Hex())
	c.Set("role", "admin")
	c.Params = gin.Params{{Key: "id", Value: b.ID.Hex()}}
	c.Request = httptest.NewRequest(http.MethodPut, "/users/"+b.ID.Hex(), bytes.NewReader([]byte(`{"email":"a@example.com"}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	h.UpdateUser(c)
	if w.Code != http.StatusConflict {
		t.Errorf("status: got %d, want 409", w.Code)
	}
}

func TestUpdateUser_EmailChange_SetsPendingAndSendsMail(t *testing.T) {
	store := newMockStore()
	rec := &recordingMailer{}
	h := handlers.NewUserHandler(store, newMapRefreshStore(), testConfig(), rec)
	u := registerUser(t, h, "Self", "self@example.com", "password12")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("userID", u.ID.Hex())
	c.Set("role", "user")
	c.Params = gin.Params{{Key: "id", Value: u.ID.Hex()}}
	c.Request = httptest.NewRequest(http.MethodPut, "/users/"+u.ID.Hex(), bytes.NewReader([]byte(`{"email":"NEW@example.com"}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	h.UpdateUser(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var got models.User
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Email != "self@example.com" {
		t.Errorf("login email: got %q, want unchanged", got.Email)
	}
	if got.PendingEmail != "new@example.com" {
		t.Errorf("pending_email: got %q", got.PendingEmail)
	}
	if rec.lastChangeTo != "new@example.com" || rec.lastChangeURL == "" {
		t.Fatalf("mailer: to=%q url=%q", rec.lastChangeTo, rec.lastChangeURL)
	}
	uu, err := url.Parse(rec.lastChangeURL)
	if err != nil {
		t.Fatal(err)
	}
	tok := uu.Query().Get("token")
	if tok == "" {
		t.Fatal("missing token in mail URL")
	}

	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/auth/confirm-email-change", bytes.NewReader([]byte(`{"token":"`+tok+`"}`)))
	c2.Request.Header.Set("Content-Type", "application/json")
	h.ConfirmEmailChange(c2)
	if w2.Code != http.StatusOK {
		t.Fatalf("confirm status %d: %s", w2.Code, w2.Body.String())
	}
	final, err := store.FindByID(context.Background(), u.ID.Hex())
	if err != nil {
		t.Fatal(err)
	}
	if final.Email != "new@example.com" || final.PendingEmail != "" {
		t.Fatalf("after confirm: email=%q pending=%q", final.Email, final.PendingEmail)
	}
	if !final.EmailVerified {
		t.Error("want email_verified true after confirm")
	}
}

func TestUpdateUser_EmailChange_Production_NotConfigured_503(t *testing.T) {
	store := newMockStore()
	cfg := testConfig()
	cfg.Env = "production"
	cfg.EmailChangeFrontendURL = ""
	h := handlers.NewUserHandler(store, newMapRefreshStore(), cfg, &recordingMailer{})
	u := registerUser(t, h, "Self", "self@example.com", "password12")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("userID", u.ID.Hex())
	c.Set("role", "user")
	c.Params = gin.Params{{Key: "id", Value: u.ID.Hex()}}
	c.Request = httptest.NewRequest(http.MethodPut, "/users/"+u.ID.Hex(), bytes.NewReader([]byte(`{"email":"other@example.com"}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	h.UpdateUser(c)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
}

func TestConfirmEmailChange_NoPending_400(t *testing.T) {
	store := newMockStore()
	h := handlers.NewUserHandler(store, newMapRefreshStore(), testConfig(), nil)
	u := registerUser(t, h, "Self", "self@example.com", "password12")
	tok, err := auth.GenerateEmailChangeToken(u.ID.Hex(), "ghost@example.com", testJWTSecret, 60)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/auth/confirm-email-change", bytes.NewReader([]byte(`{"token":"`+tok+`"}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	h.ConfirmEmailChange(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
}

func TestCancelEmailChange_ClearsPending(t *testing.T) {
	store := newMockStore()
	rec := &recordingMailer{}
	h := handlers.NewUserHandler(store, newMapRefreshStore(), testConfig(), rec)
	u := registerUser(t, h, "Self", "self@example.com", "password12")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("userID", u.ID.Hex())
	c.Set("role", "user")
	c.Params = gin.Params{{Key: "id", Value: u.ID.Hex()}}
	c.Request = httptest.NewRequest(http.MethodPut, "/users/"+u.ID.Hex(), bytes.NewReader([]byte(`{"email":"pending@example.com"}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	h.UpdateUser(c)
	if w.Code != http.StatusOK {
		t.Fatalf("update %d: %s", w.Code, w.Body.String())
	}

	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Set("userID", u.ID.Hex())
	c2.Request = httptest.NewRequest(http.MethodPost, "/me/cancel-email-change", nil)
	h.CancelEmailChange(c2)
	if w2.Code != http.StatusOK {
		t.Fatalf("cancel %d: %s", w2.Code, w2.Body.String())
	}
	var got models.User
	if err := json.Unmarshal(w2.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.PendingEmail != "" {
		t.Errorf("pending: got %q", got.PendingEmail)
	}
}

func TestResendEmailChange_NoPending_400(t *testing.T) {
	store := newMockStore()
	h := handlers.NewUserHandler(store, newMapRefreshStore(), testConfig(), &recordingMailer{})
	u := registerUser(t, h, "Self", "self@example.com", "password12")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("userID", u.ID.Hex())
	c.Request = httptest.NewRequest(http.MethodPost, "/me/resend-email-change", nil)
	h.ResendEmailChange(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateUser_NoFields(t *testing.T) {
	store := newMockStore()
	h := handlers.NewUserHandler(store, newMapRefreshStore(), testConfig(), nil)
	u := registerUser(t, h, "Solo", "solo@example.com", "password12")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("userID", u.ID.Hex())
	c.Set("role", "user")
	c.Params = gin.Params{{Key: "id", Value: u.ID.Hex()}}
	c.Request = httptest.NewRequest(http.MethodPut, "/users/"+u.ID.Hex(), bytes.NewReader([]byte(`{}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	h.UpdateUser(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", w.Code)
	}
}

func TestDeleteUser_Success(t *testing.T) {
	store := newMockStore()
	h := handlers.NewUserHandler(store, newMapRefreshStore(), testConfig(), nil)
	registerUser(t, h, "Admin", "admin@example.com", "password12")
	victim := registerUser(t, h, "Victim", "v@example.com", "password12")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: victim.ID.Hex()}}
	c.Request = httptest.NewRequest(http.MethodDelete, "/users/"+victim.ID.Hex(), nil)
	h.DeleteUser(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
}

func TestDeleteUser_NotFound(t *testing.T) {
	store := newMockStore()
	h := handlers.NewUserHandler(store, newMapRefreshStore(), testConfig(), nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	id := primitive.NewObjectID().Hex()
	c.Params = gin.Params{{Key: "id", Value: id}}
	c.Request = httptest.NewRequest(http.MethodDelete, "/users/"+id, nil)
	h.DeleteUser(c)
	if w.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", w.Code)
	}
}

type recordingMailer struct {
	lastTo         string
	lastURL        string
	lastVerifyTo   string
	lastVerifyURL  string
	lastChangeTo   string
	lastChangeURL  string
	sendErr        error
	verifySendErr  error
	changeSendErr  error
}

func (r *recordingMailer) SendPasswordReset(_ context.Context, to, resetURL string) error {
	if r.sendErr != nil {
		return r.sendErr
	}
	r.lastTo = to
	r.lastURL = resetURL
	return nil
}

func (r *recordingMailer) SendEmailVerification(_ context.Context, to, verifyURL string) error {
	if r.verifySendErr != nil {
		return r.verifySendErr
	}
	r.lastVerifyTo = to
	r.lastVerifyURL = verifyURL
	return nil
}

func (r *recordingMailer) SendEmailChange(_ context.Context, to, confirmURL string) error {
	if r.changeSendErr != nil {
		return r.changeSendErr
	}
	r.lastChangeTo = to
	r.lastChangeURL = confirmURL
	return nil
}

func TestForgotPassword_UnknownEmail_Still200(t *testing.T) {
	store := newMockStore()
	rec := &recordingMailer{}
	h := handlers.NewUserHandler(store, newMapRefreshStore(), testConfig(), rec)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/auth/forgot-password", bytes.NewReader([]byte(
		`{"email":"nobody@example.com"}`,
	)))
	c.Request.Header.Set("Content-Type", "application/json")
	h.ForgotPassword(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if rec.lastURL != "" {
		t.Error("mailer should not run for unknown email")
	}
}

func TestForgotPassword_SendsMailWhenUserExists(t *testing.T) {
	store := newMockStore()
	rec := &recordingMailer{}
	h := handlers.NewUserHandler(store, newMapRefreshStore(), testConfig(), rec)
	registerUser(t, h, "Alice", "a@example.com", "password12")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/auth/forgot-password", bytes.NewReader([]byte(
		`{"email":"a@example.com"}`,
	)))
	c.Request.Header.Set("Content-Type", "application/json")
	h.ForgotPassword(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if rec.lastTo != "a@example.com" || !strings.Contains(rec.lastURL, "token=") {
		t.Fatalf("mailer: to=%q url=%q", rec.lastTo, rec.lastURL)
	}
}

func TestForgotPassword_ProductionWithoutSMTP_503(t *testing.T) {
	store := newMockStore()
	cfg := testConfig()
	cfg.Env = "production"
	h := handlers.NewUserHandler(store, newMapRefreshStore(), cfg, &recordingMailer{})
	registerUser(t, h, "Alice", "a@example.com", "password12")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/auth/forgot-password", bytes.NewReader([]byte(
		`{"email":"a@example.com"}`,
	)))
	c.Request.Header.Set("Content-Type", "application/json")
	h.ForgotPassword(c)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
}

func TestForgotPassword_MailerError_500(t *testing.T) {
	store := newMockStore()
	rec := &recordingMailer{sendErr: errors.New("smtp down")}
	h := handlers.NewUserHandler(store, newMapRefreshStore(), testConfig(), rec)
	registerUser(t, h, "Alice", "a@example.com", "password12")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/auth/forgot-password", bytes.NewReader([]byte(
		`{"email":"a@example.com"}`,
	)))
	c.Request.Header.Set("Content-Type", "application/json")
	h.ForgotPassword(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
}

func TestForgotPassword_TestEnvIncludesResetToken(t *testing.T) {
	store := newMockStore()
	cfg := testConfig()
	cfg.Env = "test"
	h := handlers.NewUserHandler(store, newMapRefreshStore(), cfg, &recordingMailer{})
	u := registerUser(t, h, "Alice", "a@example.com", "password12")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/auth/forgot-password", bytes.NewReader([]byte(
		`{"email":"a@example.com"}`,
	)))
	c.Request.Header.Set("Content-Type", "application/json")
	h.ForgotPassword(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var resp models.ForgotPasswordResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.ResetToken == nil || *resp.ResetToken == "" {
		t.Fatal("expected reset_token in test env")
	}
	tok, err := auth.ValidatePasswordResetToken(*resp.ResetToken, testJWTSecret)
	if err != nil {
		t.Fatal(err)
	}
	if tok != u.ID.Hex() {
		t.Fatalf("token subject: got %q want %q", tok, u.ID.Hex())
	}
}

func TestResetPassword_Success(t *testing.T) {
	store := newMockStore()
	h := handlers.NewUserHandler(store, newMapRefreshStore(), testConfig(), nil)
	u := registerUser(t, h, "Alice", "a@example.com", "oldpassword1")
	tok, err := auth.GeneratePasswordResetToken(u.ID.Hex(), testJWTSecret, 60)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := `{"token":"` + tok + `","password":"newpassword1"}`
	c.Request = httptest.NewRequest(http.MethodPost, "/auth/reset-password", bytes.NewReader([]byte(body)))
	c.Request.Header.Set("Content-Type", "application/json")
	h.ResetPassword(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}

	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader([]byte(
		`{"email":"a@example.com","password":"newpassword1"}`,
	)))
	c2.Request.Header.Set("Content-Type", "application/json")
	h.Login(c2)
	if w2.Code != http.StatusOK {
		t.Fatalf("login after reset: %d %s", w2.Code, w2.Body.String())
	}
}

func TestResetPassword_InvalidToken(t *testing.T) {
	store := newMockStore()
	h := handlers.NewUserHandler(store, newMapRefreshStore(), testConfig(), nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/auth/reset-password", bytes.NewReader([]byte(
		`{"token":"not-a-valid-reset-jwt","password":"newpassword1"}`,
	)))
	c.Request.Header.Set("Content-Type", "application/json")
	h.ResetPassword(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
}

func TestResetPassword_ShortPassword(t *testing.T) {
	store := newMockStore()
	h := handlers.NewUserHandler(store, newMapRefreshStore(), testConfig(), nil)
	u := registerUser(t, h, "Alice", "a@example.com", "oldpassword1")
	tok, err := auth.GeneratePasswordResetToken(u.ID.Hex(), testJWTSecret, 60)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/auth/reset-password", bytes.NewReader([]byte(
		`{"token":"`+tok+`","password":"short"}`,
	)))
	c.Request.Header.Set("Content-Type", "application/json")
	h.ResetPassword(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
}

func TestRegister_EmailVerificationRequired_SendsVerificationMail(t *testing.T) {
	store := newMockStore()
	rec := &recordingMailer{}
	cfg := testConfig()
	cfg.EmailVerificationRequired = true
	cfg.EmailVerificationFrontendURL = "http://localhost/verify-email"
	h := handlers.NewUserHandler(store, newMapRefreshStore(), cfg, rec)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader([]byte(
		`{"name":"Alice","email":"a@example.com","password":"password12"}`,
	)))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Register(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if rec.lastVerifyTo != "a@example.com" || !strings.Contains(rec.lastVerifyURL, "token=") {
		t.Fatalf("verification mail: to=%q url=%q", rec.lastVerifyTo, rec.lastVerifyURL)
	}
	var resp models.RegisterResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.User.EmailVerified {
		t.Error("expected email_verified false before verification")
	}
}

func TestLogin_AccountLocked_AfterFailedAttempts(t *testing.T) {
	store := newMockStore()
	cfg := testConfig()
	cfg.FailedLoginMaxAttempts = 3
	cfg.FailedLoginLockoutMinutes = 15
	h := handlers.NewUserHandler(store, newMapRefreshStore(), cfg, nil)
	registerUser(t, h, "Alice", "a@example.com", "password12")

	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader([]byte(
			`{"email":"a@example.com","password":"wrongpass"}`,
		)))
		c.Request.Header.Set("Content-Type", "application/json")
		h.Login(c)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("failed attempt %d: want 401 got %d %s", i, w.Code, w.Body.String())
		}
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader([]byte(
		`{"email":"a@example.com","password":"password12"}`,
	)))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Login(c)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("correct password while locked: want 429 got %d %s", w.Code, w.Body.String())
	}
}

func TestLogin_EmailNotVerified_Forbidden(t *testing.T) {
	store := newMockStore()
	rec := &recordingMailer{}
	cfg := testConfig()
	cfg.EmailVerificationRequired = true
	cfg.EmailVerificationFrontendURL = "http://localhost/verify-email"
	h := handlers.NewUserHandler(store, newMapRefreshStore(), cfg, rec)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader([]byte(
		`{"name":"Alice","email":"a@example.com","password":"password12"}`,
	)))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Register(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", w.Code, w.Body.String())
	}

	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader([]byte(
		`{"email":"a@example.com","password":"password12"}`,
	)))
	c2.Request.Header.Set("Content-Type", "application/json")
	h.Login(c2)
	if w2.Code != http.StatusForbidden {
		t.Fatalf("login: status %d want 403, body %s", w2.Code, w2.Body.String())
	}
}

func TestVerifyEmail_ThenLoginSucceeds(t *testing.T) {
	store := newMockStore()
	rec := &recordingMailer{}
	cfg := testConfig()
	cfg.EmailVerificationRequired = true
	cfg.EmailVerificationFrontendURL = "http://localhost/verify-email"
	h := handlers.NewUserHandler(store, newMapRefreshStore(), cfg, rec)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader([]byte(
		`{"name":"Alice","email":"a@example.com","password":"password12"}`,
	)))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Register(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", w.Code, w.Body.String())
	}

	uu, err := url.Parse(rec.lastVerifyURL)
	if err != nil {
		t.Fatal(err)
	}
	tok := uu.Query().Get("token")
	if tok == "" {
		t.Fatal("missing token in verification URL")
	}

	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/auth/verify-email", bytes.NewReader([]byte(
		`{"token":"`+tok+`"}`,
	)))
	c2.Request.Header.Set("Content-Type", "application/json")
	h.VerifyEmail(c2)
	if w2.Code != http.StatusOK {
		t.Fatalf("verify: %d %s", w2.Code, w2.Body.String())
	}

	i := store.byEmail["a@example.com"]
	if !store.users[i].EmailVerified {
		t.Error("user should be marked verified in store")
	}

	w3 := httptest.NewRecorder()
	c3, _ := gin.CreateTestContext(w3)
	c3.Request = httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader([]byte(
		`{"email":"a@example.com","password":"password12"}`,
	)))
	c3.Request.Header.Set("Content-Type", "application/json")
	h.Login(c3)
	if w3.Code != http.StatusOK {
		t.Fatalf("login after verify: %d %s", w3.Code, w3.Body.String())
	}
}
