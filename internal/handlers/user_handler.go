package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go-microservice/internal/auth"
	"go-microservice/internal/config"
	"go-microservice/internal/models"
	"go-microservice/internal/repository"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/crypto/bcrypt"
)

// AuthMailer delivers transactional auth emails (password reset, email verification).
type AuthMailer interface {
	SendPasswordReset(ctx context.Context, toEmail, resetURL string) error
	SendEmailVerification(ctx context.Context, toEmail, verifyURL string) error
}

type noopMailer struct{}

func (noopMailer) SendPasswordReset(context.Context, string, string) error { return nil }

func (noopMailer) SendEmailVerification(context.Context, string, string) error { return nil }

type userStore interface {
	Count(ctx context.Context) (int64, error)
	Create(ctx context.Context, user *models.User) error
	FindByEmail(ctx context.Context, email string) (*models.User, error)
	FindAll(ctx context.Context) ([]models.User, error)
	FindByID(ctx context.Context, id string) (*models.User, error)
	Update(ctx context.Context, id string, update bson.M) (*models.User, error)
	Delete(ctx context.Context, id string) error
}

type refreshSessionStore interface {
	Insert(ctx context.Context, userID primitive.ObjectID, tokenHash, familyID string, expiresAt time.Time) error
	ConsumeAndRotate(ctx context.Context, presentedHash, newHash string, newExpires time.Time) (*repository.RefreshTokenDoc, error)
	DeleteByHash(ctx context.Context, tokenHash string) (bool, error)
	DeleteAllForUser(ctx context.Context, userID primitive.ObjectID) (int64, error)
}

type UserHandler struct {
	repo    userStore
	refresh refreshSessionStore
	cfg     *config.Config
	mailer  AuthMailer
}

func NewUserHandler(repo userStore, refresh refreshSessionStore, cfg *config.Config, mailer AuthMailer) *UserHandler {
	if mailer == nil {
		mailer = noopMailer{}
	}
	if refresh == nil {
		panic("refresh session store is required")
	}
	return &UserHandler{repo: repo, refresh: refresh, cfg: cfg, mailer: mailer}
}

func passwordResetLink(base, token string) string {
	b := strings.TrimSuffix(strings.TrimSpace(base), "/")
	return b + "?token=" + url.QueryEscape(token)
}

func emailVerificationLink(base, token string) string {
	return passwordResetLink(base, token)
}

// Register godoc
// @Summary      Register a new user
// @Description  Creates a new user account. The first user in the database becomes "admin"; everyone else is "user". Client-supplied roles are ignored. When EMAIL_VERIFICATION_REQUIRED is true, the user is created with email_verified false and a verification email is sent; production requires SMTP and EMAIL_VERIFICATION_FRONTEND_URL.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      models.CreateUserRequest  true  "Registration details"
// @Success      201   {object}  models.RegisterResponse
// @Failure      400   {object}  models.ErrorResponse  "Validation error"
// @Failure      409   {object}  models.ErrorResponse  "Email already exists"
// @Failure      503   {object}  models.ErrorResponse  "Email verification enabled but not configured (production)"
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

	emailVerified := true
	if h.cfg.EmailVerificationRequired {
		emailVerified = false
	}

	if h.cfg.EmailVerificationRequired && h.cfg.IsProduction() {
		if strings.TrimSpace(h.cfg.SMTPHost) == "" || strings.TrimSpace(h.cfg.EmailVerificationFrontendURL) == "" {
			c.JSON(http.StatusServiceUnavailable, models.ErrorResponse{
				Error: "email verification is enabled but not configured (set SMTP_HOST, SMTP_FROM, and EMAIL_VERIFICATION_FRONTEND_URL)",
			})
			return
		}
	}

	user := &models.User{
		Name:          req.Name,
		Email:         req.Email,
		Password:      string(hashed),
		Role:          role,
		EmailVerified: emailVerified,
	}

	if err := h.repo.Create(c.Request.Context(), user); err != nil {
		if errors.Is(err, repository.ErrDuplicateEmail) {
			c.JSON(http.StatusConflict, models.ErrorResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "failed to create user"})
		return
	}

	if !h.cfg.EmailVerificationRequired {
		c.JSON(http.StatusCreated, models.RegisterResponse{Message: "user registered successfully", User: *user})
		return
	}

	tok, err := auth.GenerateEmailVerificationToken(user.ID.Hex(), h.cfg.JWTSecret, h.cfg.EmailVerificationTokenMinutes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "failed to generate verification token"})
		return
	}
	link := emailVerificationLink(h.cfg.EmailVerificationFrontendURL, tok)
	if err := h.mailer.SendEmailVerification(c.Request.Context(), user.Email, link); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "failed to send verification email"})
		return
	}

	resp := models.RegisterResponse{
		Message: "User registered. Check your email to verify your address before signing in.",
		User:    *user,
	}
	if h.cfg.IsTestEnv() {
		resp.VerificationToken = &tok
	}
	c.JSON(http.StatusCreated, resp)
}

// Login godoc
// @Summary      Login
// @Description  Authenticates a user and returns a short-lived access JWT plus a rotating refresh token.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      models.LoginRequest  true  "Login credentials"
// @Success      200   {object}  models.LoginResponse
// @Failure      400   {object}  models.ErrorResponse  "Validation error"
// @Failure      401   {object}  models.ErrorResponse  "Invalid credentials"
// @Failure      403   {object}  models.ErrorResponse  "Email not verified (when verification is required)"
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

	if h.cfg.EmailVerificationRequired && !user.EmailVerified {
		c.JSON(http.StatusForbidden, models.ErrorResponse{Error: "email address not verified"})
		return
	}

	resp, err := h.createSession(c, user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "failed to create session"})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *UserHandler) createSession(c *gin.Context, user *models.User) (models.LoginResponse, error) {
	access, err := auth.GenerateAccessToken(user.ID.Hex(), user.Email, user.Role, h.cfg.JWTSecret, h.cfg.JWTAccessExpireMinutes)
	if err != nil {
		return models.LoginResponse{}, err
	}
	plain, hash, err := auth.NewRefreshTokenPair()
	if err != nil {
		return models.LoginResponse{}, err
	}
	familyID := primitive.NewObjectID().Hex()
	expiresAt := time.Now().Add(time.Duration(h.cfg.JWTRefreshExpireHours) * time.Hour)
	if err := h.refresh.Insert(c.Request.Context(), user.ID, hash, familyID, expiresAt); err != nil {
		return models.LoginResponse{}, err
	}
	expiresIn := h.cfg.JWTAccessExpireMinutes * 60
	if expiresIn < 1 {
		expiresIn = 1
	}
	return models.LoginResponse{
		AccessToken:  access,
		RefreshToken: plain,
		ExpiresIn:    expiresIn,
		Token:        access,
		User:         *user,
	}, nil
}

// Refresh godoc
// @Summary      Refresh tokens
// @Description  Exchanges a valid refresh token for a new access token and a new refresh token (rotation).
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      models.RefreshTokenBody  true  "Current refresh token"
// @Success      200   {object}  models.RefreshResponse
// @Failure      400   {object}  models.ErrorResponse
// @Failure      401   {object}  models.ErrorResponse
// @Failure      500   {object}  models.ErrorResponse
// @Router       /auth/refresh [post]
func (h *UserHandler) Refresh(c *gin.Context) {
	var req models.RefreshTokenBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	presentedHash := auth.HashRefreshToken(req.RefreshToken)
	newPlain, newHash, err := auth.NewRefreshTokenPair()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "failed to issue refresh token"})
		return
	}
	newExp := time.Now().Add(time.Duration(h.cfg.JWTRefreshExpireHours) * time.Hour)
	old, err := h.refresh.ConsumeAndRotate(c.Request.Context(), presentedHash, newHash, newExp)
	if err != nil {
		if errors.Is(err, repository.ErrInvalidRefreshToken) {
			c.JSON(http.StatusUnauthorized, models.ErrorResponse{Error: "invalid or expired refresh token"})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	user, err := h.repo.FindByID(c.Request.Context(), old.UserID.Hex())
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{Error: "invalid or expired refresh token"})
		return
	}

	access, err := auth.GenerateAccessToken(user.ID.Hex(), user.Email, user.Role, h.cfg.JWTSecret, h.cfg.JWTAccessExpireMinutes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "failed to generate access token"})
		return
	}
	expiresIn := h.cfg.JWTAccessExpireMinutes * 60
	if expiresIn < 1 {
		expiresIn = 1
	}
	c.JSON(http.StatusOK, models.RefreshResponse{
		AccessToken:  access,
		RefreshToken: newPlain,
		ExpiresIn:    expiresIn,
		Token:        access,
	})
}

// Logout godoc
// @Summary      Logout
// @Description  Revokes the given refresh token (client should discard access + refresh locally).
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      models.RefreshTokenBody  true  "Refresh token to revoke"
// @Success      200   {object}  models.MessageResponse
// @Failure      400   {object}  models.ErrorResponse
// @Failure      500   {object}  models.ErrorResponse
// @Router       /auth/logout [post]
func (h *UserHandler) Logout(c *gin.Context) {
	var req models.RefreshTokenBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}
	hash := auth.HashRefreshToken(req.RefreshToken)
	if _, err := h.refresh.DeleteByHash(c.Request.Context(), hash); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.MessageResponse{Message: "logged out"})
}

const forgotPasswordAck = "If an account exists for this email, you will receive reset instructions."

// ForgotPassword godoc
// @Summary      Request password reset
// @Description  Sends a reset link to the email when an account exists. Always returns the same message (does not reveal whether the email is registered). In production, SMTP and PASSWORD_RESET_FRONTEND_URL must be configured.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      models.ForgotPasswordRequest  true  "Email address"
// @Success      200   {object}  models.ForgotPasswordResponse
// @Failure      400   {object}  models.ErrorResponse
// @Failure      503   {object}  models.ErrorResponse  "Password reset not configured (production)"
// @Failure      500   {object}  models.ErrorResponse
// @Router       /auth/forgot-password [post]
func (h *UserHandler) ForgotPassword(c *gin.Context) {
	if h.cfg.IsProduction() {
		if strings.TrimSpace(h.cfg.SMTPHost) == "" || strings.TrimSpace(h.cfg.PasswordResetFrontendURL) == "" {
			c.JSON(http.StatusServiceUnavailable, models.ErrorResponse{
				Error: "password reset is not configured (set SMTP_HOST, SMTP_FROM, and PASSWORD_RESET_FRONTEND_URL)",
			})
			return
		}
	}

	var req models.ForgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	user, err := h.repo.FindByEmail(c.Request.Context(), req.Email)
	if err != nil {
		c.JSON(http.StatusOK, models.ForgotPasswordResponse{Message: forgotPasswordAck})
		return
	}

	token, err := auth.GeneratePasswordResetToken(user.ID.Hex(), h.cfg.JWTSecret, h.cfg.PasswordResetTokenMinutes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "failed to generate reset token"})
		return
	}

	link := passwordResetLink(h.cfg.PasswordResetFrontendURL, token)
	if err := h.mailer.SendPasswordReset(c.Request.Context(), user.Email, link); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "failed to send reset email"})
		return
	}

	resp := models.ForgotPasswordResponse{Message: forgotPasswordAck}
	if h.cfg.IsTestEnv() {
		resp.ResetToken = &token
	}
	c.JSON(http.StatusOK, resp)
}

// ResetPassword godoc
// @Summary      Complete password reset
// @Description  Sets a new password using the short-lived token from the reset email.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      models.ResetPasswordRequest  true  "Token and new password"
// @Success      200   {object}  models.MessageResponse
// @Failure      400   {object}  models.ErrorResponse
// @Failure      404   {object}  models.ErrorResponse
// @Failure      500   {object}  models.ErrorResponse
// @Router       /auth/reset-password [post]
func (h *UserHandler) ResetPassword(c *gin.Context) {
	var req models.ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	userID, err := auth.ValidatePasswordResetToken(req.Token, h.cfg.JWTSecret)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid or expired reset token"})
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "failed to hash password"})
		return
	}

	_, err = h.repo.Update(c.Request.Context(), userID, bson.M{"password": string(hashed)})
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: err.Error()})
		return
	}

	if oid, err := primitive.ObjectIDFromHex(userID); err == nil {
		_, _ = h.refresh.DeleteAllForUser(c.Request.Context(), oid)
	}

	c.JSON(http.StatusOK, models.MessageResponse{Message: "password updated successfully"})
}

const resendVerificationAck = "If an account exists and needs verification, you will receive an email."

// VerifyEmail godoc
// @Summary      Verify email address
// @Description  Confirms email ownership using the token from the registration email. Public endpoint.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      models.VerifyEmailRequest  true  "Verification token from email"
// @Success      200   {object}  models.MessageResponse
// @Failure      400   {object}  models.ErrorResponse
// @Failure      404   {object}  models.ErrorResponse
// @Router       /auth/verify-email [post]
func (h *UserHandler) VerifyEmail(c *gin.Context) {
	var req models.VerifyEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	userID, err := auth.ValidateEmailVerificationToken(req.Token, h.cfg.JWTSecret)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid or expired verification token"})
		return
	}

	user, err := h.repo.FindByID(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: err.Error()})
		return
	}

	if user.EmailVerified {
		c.JSON(http.StatusOK, models.MessageResponse{Message: "email already verified"})
		return
	}

	if _, err := h.repo.Update(c.Request.Context(), userID, bson.M{"email_verified": true}); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, models.MessageResponse{Message: "email verified successfully"})
}

// ResendVerification godoc
// @Summary      Resend verification email
// @Description  Sends a new verification link when the account exists and is not yet verified. Same response whether or not the email is registered (privacy). Requires EMAIL_VERIFICATION_REQUIRED; in production, SMTP and EMAIL_VERIFICATION_FRONTEND_URL must be set.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      models.ResendVerificationRequest  true  "Email address"
// @Success      200   {object}  models.MessageResponse
// @Failure      400   {object}  models.ErrorResponse
// @Failure      503   {object}  models.ErrorResponse
// @Failure      500   {object}  models.ErrorResponse
// @Router       /auth/resend-verification [post]
func (h *UserHandler) ResendVerification(c *gin.Context) {
	if !h.cfg.EmailVerificationRequired {
		c.JSON(http.StatusOK, models.MessageResponse{Message: resendVerificationAck})
		return
	}

	if h.cfg.IsProduction() {
		if strings.TrimSpace(h.cfg.SMTPHost) == "" || strings.TrimSpace(h.cfg.EmailVerificationFrontendURL) == "" {
			c.JSON(http.StatusServiceUnavailable, models.ErrorResponse{
				Error: "email verification is not configured (set SMTP_HOST, SMTP_FROM, and EMAIL_VERIFICATION_FRONTEND_URL)",
			})
			return
		}
	}

	var req models.ResendVerificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	user, err := h.repo.FindByEmail(c.Request.Context(), req.Email)
	if err != nil {
		c.JSON(http.StatusOK, models.MessageResponse{Message: resendVerificationAck})
		return
	}

	if user.EmailVerified {
		c.JSON(http.StatusOK, models.MessageResponse{Message: resendVerificationAck})
		return
	}

	tok, err := auth.GenerateEmailVerificationToken(user.ID.Hex(), h.cfg.JWTSecret, h.cfg.EmailVerificationTokenMinutes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "failed to generate verification token"})
		return
	}
	link := emailVerificationLink(h.cfg.EmailVerificationFrontendURL, tok)
	if err := h.mailer.SendEmailVerification(c.Request.Context(), user.Email, link); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "failed to send verification email"})
		return
	}

	c.JSON(http.StatusOK, models.MessageResponse{Message: resendVerificationAck})
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

	existing, err := h.repo.FindByID(c.Request.Context(), targetID)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: err.Error()})
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
	if req.Email != "" && req.Email != existing.Email {
		update["email"] = req.Email
		if h.cfg.EmailVerificationRequired {
			update["email_verified"] = false
		}
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

	if h.cfg.EmailVerificationRequired {
		if em, ok := update["email"].(string); ok && em != "" && em != existing.Email {
			tok, terr := auth.GenerateEmailVerificationToken(user.ID.Hex(), h.cfg.JWTSecret, h.cfg.EmailVerificationTokenMinutes)
			if terr == nil {
				link := emailVerificationLink(h.cfg.EmailVerificationFrontendURL, tok)
				_ = h.mailer.SendEmailVerification(c.Request.Context(), user.Email, link)
			}
		}
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
	id := c.Param("id")
	if err := h.repo.Delete(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: err.Error()})
		return
	}
	if oid, err := primitive.ObjectIDFromHex(id); err == nil {
		_, _ = h.refresh.DeleteAllForUser(c.Request.Context(), oid)
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
