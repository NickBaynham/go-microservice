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
	count   int64
	users   []models.User
	byEmail map[string]int // index in users
}

func newMockStore() *mockUserStore {
	return &mockUserStore{byEmail: make(map[string]int)}
}

func (m *mockUserStore) Count(ctx context.Context) (int64, error) {
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
	if name, ok := update["name"].(string); ok {
		u.Name = name
	}
	if email, ok := update["email"].(string); ok {
		u.Email = email
	}
	if role, ok := update["role"].(string); ok {
		u.Role = role
	}
	idx := m.byEmail[u.Email]
	m.users[idx] = *u
	return u, nil
}

func (m *mockUserStore) Delete(ctx context.Context, id string) error {
	u, err := m.FindByID(ctx, id)
	if err != nil {
		return err
	}
	delete(m.byEmail, u.Email)
	// simplify: rebuild slice
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
