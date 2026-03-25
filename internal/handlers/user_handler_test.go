package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
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
		JWTSecret:      testJWTSecret,
		JWTExpireHours: "24",
		Env:            "development",
	}
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
	m.users[idx] = *u
	return u, nil
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
	h := handlers.NewUserHandler(store, testConfig())

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
	h := handlers.NewUserHandler(store, testConfig())

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
	h := handlers.NewUserHandler(store, testConfig())
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
	h := handlers.NewUserHandler(store, testConfig())

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
	h := handlers.NewUserHandler(store, testConfig())
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
	if resp.Token == "" {
		t.Error("expected token")
	}
	if resp.User.ID != u.ID {
		t.Errorf("user id mismatch")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	store := newMockStore()
	h := handlers.NewUserHandler(store, testConfig())
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
	h := handlers.NewUserHandler(store, testConfig())

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
	cfg.JWTExpireHours = "not-a-number"
	h := handlers.NewUserHandler(store, cfg)
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
	h := handlers.NewUserHandler(store, testConfig())
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
	h := handlers.NewUserHandler(store, testConfig())
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
	h := handlers.NewUserHandler(store, testConfig())
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
	h := handlers.NewUserHandler(store, testConfig())

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
	h := handlers.NewUserHandler(store, testConfig())
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
	h := handlers.NewUserHandler(store, testConfig())

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
	h := handlers.NewUserHandler(store, testConfig())
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
	h := handlers.NewUserHandler(store, testConfig())
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
	h := handlers.NewUserHandler(store, testConfig())
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
	h := handlers.NewUserHandler(store, testConfig())
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
	h := handlers.NewUserHandler(store, testConfig())
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

func TestUpdateUser_NoFields(t *testing.T) {
	store := newMockStore()
	h := handlers.NewUserHandler(store, testConfig())
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
	h := handlers.NewUserHandler(store, testConfig())
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
	h := handlers.NewUserHandler(store, testConfig())

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
