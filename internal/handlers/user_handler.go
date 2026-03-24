package handlers

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go-microservice/internal/auth"
	"go-microservice/internal/config"
	"go-microservice/internal/models"
	"go-microservice/internal/repository"
	"go.mongodb.org/mongo-driver/bson"
	"golang.org/x/crypto/bcrypt"
)

type userStore interface {
	Count(ctx context.Context) (int64, error)
	Create(ctx context.Context, user *models.User) error
	FindByEmail(ctx context.Context, email string) (*models.User, error)
	FindAll(ctx context.Context) ([]models.User, error)
	FindByID(ctx context.Context, id string) (*models.User, error)
	Update(ctx context.Context, id string, update bson.M) (*models.User, error)
	Delete(ctx context.Context, id string) error
}

type UserHandler struct {
	repo userStore
	cfg  *config.Config
}

func NewUserHandler(repo userStore, cfg *config.Config) *UserHandler {
	return &UserHandler{repo: repo, cfg: cfg}
}

// Register godoc
// @Summary      Register a new user
// @Description  Creates a new user account. The first user in the database becomes "admin"; everyone else is "user". Client-supplied roles are ignored.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      models.CreateUserRequest  true  "Registration details"
// @Success      201   {object}  models.RegisterResponse
// @Failure      400   {object}  models.ErrorResponse  "Validation error"
// @Failure      409   {object}  models.ErrorResponse  "Email already exists"
// @Failure      500   {object}  models.ErrorResponse  "Internal server error"
// @Router       /auth/register [post]
func (h *UserHandler) Register(c *gin.Context) {
	var req models.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "failed to hash password"})
		return
	}

	count, err := h.repo.Count(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "failed to check user count"})
		return
	}

	role := "user"
	if count == 0 {
		role = "admin"
	}

	user := &models.User{
		Name:     req.Name,
		Email:    req.Email,
		Password: string(hashed),
		Role:     role,
	}

	if err := h.repo.Create(c.Request.Context(), user); err != nil {
		if errors.Is(err, repository.ErrDuplicateEmail) {
			c.JSON(http.StatusConflict, models.ErrorResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "failed to create user"})
		return
	}

	c.JSON(http.StatusCreated, models.RegisterResponse{Message: "user registered successfully", User: *user})
}

// Login godoc
// @Summary      Login
// @Description  Authenticates a user and returns a signed JWT token.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      models.LoginRequest  true  "Login credentials"
// @Success      200   {object}  models.LoginResponse
// @Failure      400   {object}  models.ErrorResponse  "Validation error"
// @Failure      401   {object}  models.ErrorResponse  "Invalid credentials"
// @Failure      500   {object}  models.ErrorResponse  "Internal server error"
// @Router       /auth/login [post]
func (h *UserHandler) Login(c *gin.Context) {
	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	user, err := h.repo.FindByEmail(c.Request.Context(), req.Email)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{Error: "invalid credentials"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{Error: "invalid credentials"})
		return
	}

	token, err := auth.GenerateToken(user.ID.Hex(), user.Email, user.Role, h.cfg.JWTSecret, h.cfg.JWTExpireHours)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, models.LoginResponse{Token: token, User: *user})
}

// ListUsers godoc
// @Summary      List all users
// @Description  Returns all users in the system. Admin only.
// @Tags         users
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  models.ListUsersResponse
// @Failure      401  {object}  models.ErrorResponse  "Unauthorized"
// @Failure      403  {object}  models.ErrorResponse  "Admin access required"
// @Failure      500  {object}  models.ErrorResponse  "Internal server error"
// @Router       /users [get]
func (h *UserHandler) ListUsers(c *gin.Context) {
	users, err := h.repo.FindAll(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.ListUsersResponse{Users: users, Count: len(users)})
}

// GetUser godoc
// @Summary      Get a user by ID
// @Description  Returns a single user by their ID. Admin only.
// @Tags         users
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      string  true  "User ID"
// @Success      200  {object}  models.User
// @Failure      400  {object}  models.ErrorResponse  "Invalid ID"
// @Failure      401  {object}  models.ErrorResponse  "Unauthorized"
// @Failure      403  {object}  models.ErrorResponse  "Admin access required"
// @Failure      404  {object}  models.ErrorResponse  "User not found"
// @Router       /users/{id} [get]
func (h *UserHandler) GetUser(c *gin.Context) {
	user, err := h.repo.FindByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, user)
}

// UpdateUser godoc
// @Summary      Update a user
// @Description  Updates a user's profile. Users can update themselves; admins can update anyone and change roles.
// @Tags         users
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      string                   true  "User ID"
// @Param        body  body      models.UpdateUserRequest  true  "Fields to update"
// @Success      200   {object}  models.User
// @Failure      400   {object}  models.ErrorResponse  "Validation error or no fields to update"
// @Failure      401   {object}  models.ErrorResponse  "Unauthorized"
// @Failure      403   {object}  models.ErrorResponse  "Forbidden"
// @Failure      404   {object}  models.ErrorResponse  "User not found"
// @Router       /users/{id} [put]
func (h *UserHandler) UpdateUser(c *gin.Context) {
	callerID, _ := c.Get("userID")
	callerRole, _ := c.Get("role")
	targetID := c.Param("id")

	if callerID != targetID && callerRole != "admin" {
		c.JSON(http.StatusForbidden, models.ErrorResponse{Error: "cannot update another user's profile"})
		return
	}

	var req models.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	update := bson.M{}
	if req.Name != "" {
		update["name"] = req.Name
	}
	if req.Email != "" {
		update["email"] = req.Email
	}
	if req.Role != "" && callerRole == "admin" {
		update["role"] = req.Role
	}

	if len(update) == 0 {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "no fields to update"})
		return
	}

	user, err := h.repo.Update(c.Request.Context(), targetID, update)
	if err != nil {
		if errors.Is(err, repository.ErrDuplicateEmail) {
			c.JSON(http.StatusConflict, models.ErrorResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, user)
}

// DeleteUser godoc
// @Summary      Delete a user
// @Description  Permanently deletes a user by ID. Admin only.
// @Tags         users
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      string  true  "User ID"
// @Success      200  {object}  models.MessageResponse
// @Failure      401  {object}  models.ErrorResponse  "Unauthorized"
// @Failure      403  {object}  models.ErrorResponse  "Admin access required"
// @Failure      404  {object}  models.ErrorResponse  "User not found"
// @Router       /users/{id} [delete]
func (h *UserHandler) DeleteUser(c *gin.Context) {
	if err := h.repo.Delete(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.MessageResponse{Message: "user deleted successfully"})
}

// GetMe godoc
// @Summary      Get current user
// @Description  Returns the profile of the currently authenticated user.
// @Tags         users
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  models.User
// @Failure      401  {object}  models.ErrorResponse  "Unauthorized"
// @Failure      404  {object}  models.ErrorResponse  "User not found"
// @Router       /me [get]
func (h *UserHandler) GetMe(c *gin.Context) {
	userID, _ := c.Get("userID")
	user, err := h.repo.FindByID(c.Request.Context(), userID.(string))
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, user)
}
