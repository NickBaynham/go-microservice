package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go-microservice/internal/abuse"
	"go-microservice/internal/auth"
	"go-microservice/internal/config"
	"go-microservice/internal/models"
	"go-microservice/internal/repository"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/crypto/bcrypt"
)

// AuthMailer delivers transactional auth emails (password reset, email verification, email change).
type AuthMailer interface {
	SendPasswordReset(ctx context.Context, toEmail, resetURL string) error
	SendEmailVerification(ctx context.Context, toEmail, verifyURL string) error
	SendEmailChange(ctx context.Context, toEmail, confirmURL string) error
}

type noopMailer struct{}

func (noopMailer) SendPasswordReset(context.Context, string, string) error { return nil }

func (noopMailer) SendEmailVerification(context.Context, string, string) error { return nil }

func (noopMailer) SendEmailChange(context.Context, string, string) error { return nil }

type userStore interface {
	Count(ctx context.Context) (int64, error)
	Create(ctx context.Context, user *models.User) error
	FindByEmail(ctx context.Context, email string) (*models.User, error)
	FindAll(ctx context.Context) ([]models.User, error)
	FindByID(ctx context.Context, id string) (*models.User, error)
	Update(ctx context.Context, id string, update bson.M) (*models.User, error)
	Delete(ctx context.Context, id string) error
	IncrementFailedLogin(ctx context.Context, userID string, maxAttempts int, lockout time.Duration) error
	ClearLoginLockout(ctx context.Context, userID string) error
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

func normalizeEmailAddr(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// emailChangeDeliveryReady is false in production when SMTP or the confirm-URL base is missing.
func (h *UserHandler) emailChangeDeliveryReady(c *gin.Context) bool {
	if !h.cfg.IsProduction() {
		return true
	}
	if strings.TrimSpace(h.cfg.SMTPHost) == "" || strings.TrimSpace(h.cfg.SMTPFrom) == "" {
		c.JSON(http.StatusServiceUnavailable, models.ErrorResponse{
			Error: "email change is not configured (set SMTP_HOST, SMTP_FROM, and EMAIL_CHANGE_FRONTEND_URL)",
		})
		return false
	}
	if strings.TrimSpace(h.cfg.EmailChangeFrontendURL) == "" {
		c.JSON(http.StatusServiceUnavailable, models.ErrorResponse{
			Error: "email change is not configured (set EMAIL_CHANGE_FRONTEND_URL)",
		})
		return false
	}
	return true
}

func (h *UserHandler) verifyTurnstileOrAbort(c *gin.Context, token string) bool {
	if !h.cfg.TurnstileEnabled() {
		return true
	}
	ok, err := abuse.VerifyTurnstile(c.Request.Context(), h.cfg.TurnstileSecretKey, token, c.ClientIP())
	if err != nil {
		AuthAudit(c, slog.LevelWarn, "turnstile_error", slog.String("detail", err.Error()))
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "captcha verification failed"})
		return false
	}
	if !ok {
		AuthAudit(c, slog.LevelWarn, "turnstile_rejected")
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "captcha verification failed"})
		return false
	}
	return true
}

// Register godoc
// @Summary      Register a new user
// @Description  Creates a new user account. The first user in the database becomes "admin"; everyone else is "user". Client-supplied roles are ignored. When EMAIL_VERIFICATION_REQUIRED is true, the user is created with email_verified false and a verification email is sent; production requires SMTP and EMAIL_VERIFICATION_FRONTEND_URL.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      models.CreateUserRequest  true  "Registration details"
// @Success      201   {object}  models.RegisterResponse
// @Failure      400   {object}  models.ErrorResponse  "Validation error or captcha failed when Turnstile is configured"
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

	if !h.verifyTurnstileOrAbort(c, req.TurnstileToken) {
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
		AuthAudit(c, slog.LevelInfo, "register_success", slog.String("email", req.Email), slog.String("user_id", user.ID.Hex()))
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
	AuthAudit(c, slog.LevelInfo, "register_success", slog.String("email", req.Email), slog.String("user_id", user.ID.Hex()))
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
// @Failure      400   {object}  models.ErrorResponse  "Captcha failed when Turnstile is configured"
// @Failure      401   {object}  models.ErrorResponse  "Invalid credentials"
// @Failure      403   {object}  models.ErrorResponse  "Email not verified (when verification is required)"
// @Failure      429   {object}  models.ErrorResponse  "Account temporarily locked after failed logins"
// @Failure      500   {object}  models.ErrorResponse  "Internal server error"
// @Router       /auth/login [post]
func (h *UserHandler) Login(c *gin.Context) {
	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	if !h.verifyTurnstileOrAbort(c, req.TurnstileToken) {
		return
	}

	user, err := h.repo.FindByEmail(c.Request.Context(), req.Email)
	if err != nil {
		AuthAudit(c, slog.LevelWarn, "login_failure", slog.String("email", req.Email), slog.String("reason", "unknown_email"))
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{Error: "invalid credentials"})
		return
	}

	if h.cfg.LoginLockoutEnabled() && user.LockedUntil != nil && time.Now().Before(*user.LockedUntil) {
		AuthAudit(c, slog.LevelWarn, "login_blocked", slog.String("email", req.Email), slog.String("user_id", user.ID.Hex()))
		c.JSON(http.StatusTooManyRequests, models.ErrorResponse{Error: "too many failed login attempts; try again later"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		if h.cfg.LoginLockoutEnabled() {
			_ = h.repo.IncrementFailedLogin(c.Request.Context(), user.ID.Hex(), h.cfg.FailedLoginMaxAttempts,
				time.Duration(h.cfg.FailedLoginLockoutMinutes)*time.Minute)
		}
		AuthAudit(c, slog.LevelWarn, "login_failure", slog.String("email", req.Email), slog.String("reason", "bad_password"))
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{Error: "invalid credentials"})
		return
	}

	_ = h.repo.ClearLoginLockout(c.Request.Context(), user.ID.Hex())

	if h.cfg.EmailVerificationRequired && !user.EmailVerified {
		AuthAudit(c, slog.LevelWarn, "login_failure", slog.String("email", req.Email), slog.String("reason", "email_unverified"))
		c.JSON(http.StatusForbidden, models.ErrorResponse{Error: "email address not verified"})
		return
	}

	resp, err := h.createSession(c, user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "failed to create session"})
		return
	}

	AuthAudit(c, slog.LevelInfo, "login_success", slog.String("email", req.Email), slog.String("user_id", user.ID.Hex()))
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

	_ = h.repo.ClearLoginLockout(c.Request.Context(), userID)

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

// ConfirmEmailChange godoc
// @Summary      Confirm pending email change
// @Description  Applies a new email after the user follows the link sent to the pending address. Invalidates refresh sessions.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      models.ConfirmEmailChangeRequest  true  "Token from the email change message"
// @Success      200   {object}  models.MessageResponse
// @Failure      400   {object}  models.ErrorResponse
// @Failure      409   {object}  models.ErrorResponse  "New email already in use"
// @Failure      404   {object}  models.ErrorResponse
// @Router       /auth/confirm-email-change [post]
func (h *UserHandler) ConfirmEmailChange(c *gin.Context) {
	var req models.ConfirmEmailChangeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	userID, newEmail, err := auth.ValidateEmailChangeToken(req.Token, h.cfg.JWTSecret)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid or expired email change token"})
		return
	}

	user, err := h.repo.FindByID(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: err.Error()})
		return
	}

	if normalizeEmailAddr(user.PendingEmail) != newEmail {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "no pending email change for this token"})
		return
	}

	other, ferr := h.repo.FindByEmail(c.Request.Context(), newEmail)
	if ferr == nil && other.ID != user.ID {
		c.JSON(http.StatusConflict, models.ErrorResponse{Error: repository.ErrDuplicateEmail.Error()})
		return
	}

	if _, err := h.repo.Update(c.Request.Context(), userID, bson.M{
		"email":          newEmail,
		"email_verified": true,
		"pending_email":  "",
	}); err != nil {
		if errors.Is(err, repository.ErrDuplicateEmail) {
			c.JSON(http.StatusConflict, models.ErrorResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	if oid, err := primitive.ObjectIDFromHex(userID); err == nil {
		_, _ = h.refresh.DeleteAllForUser(c.Request.Context(), oid)
	}

	AuthAudit(c, slog.LevelInfo, "email_change_confirmed", slog.String("user_id", userID), slog.String("email", newEmail))
	c.JSON(http.StatusOK, models.MessageResponse{Message: "email address updated successfully"})
}

// ResendEmailChange godoc
// @Summary      Resend email change confirmation
// @Description  Sends another confirmation link to the pending address when one is set.
// @Tags         users
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200   {object}  models.MessageResponse
// @Failure      400   {object}  models.ErrorResponse
// @Failure      401   {object}  models.ErrorResponse
// @Failure      503   {object}  models.ErrorResponse
// @Failure      500   {object}  models.ErrorResponse
// @Router       /me/resend-email-change [post]
func (h *UserHandler) ResendEmailChange(c *gin.Context) {
	userID, _ := c.Get("userID")
	uid, _ := userID.(string)

	u, err := h.repo.FindByID(c.Request.Context(), uid)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: err.Error()})
		return
	}
	pending := normalizeEmailAddr(u.PendingEmail)
	if pending == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "no pending email change"})
		return
	}
	if !h.emailChangeDeliveryReady(c) {
		return
	}

	tok, terr := auth.GenerateEmailChangeToken(uid, pending, h.cfg.JWTSecret, h.cfg.EmailChangeTokenMinutes)
	if terr != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "failed to generate email change token"})
		return
	}
	link := passwordResetLink(h.cfg.EmailChangeFrontendURL, tok)
	if err := h.mailer.SendEmailChange(c.Request.Context(), pending, link); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "failed to send email change confirmation"})
		return
	}

	c.JSON(http.StatusOK, models.MessageResponse{Message: "confirmation email sent"})
}

// CancelEmailChange godoc
// @Summary      Cancel pending email change
// @Description  Clears pending_email without changing the login address.
// @Tags         users
// @Produce      json
// @Security     BearerAuth
// @Success      200   {object}  models.User
// @Failure      400   {object}  models.ErrorResponse
// @Failure      401   {object}  models.ErrorResponse
// @Router       /me/cancel-email-change [post]
func (h *UserHandler) CancelEmailChange(c *gin.Context) {
	userID, _ := c.Get("userID")
	uid, _ := userID.(string)

	u, err := h.repo.FindByID(c.Request.Context(), uid)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: err.Error()})
		return
	}
	if normalizeEmailAddr(u.PendingEmail) == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "no pending email change"})
		return
	}

	user, err := h.repo.Update(c.Request.Context(), uid, bson.M{"pending_email": ""})
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	AuthAudit(c, slog.LevelInfo, "email_change_cancelled", slog.String("user_id", uid))
	c.JSON(http.StatusOK, user)
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
// @Description  Updates a user's profile. Users can update themselves; admins can update anyone and change roles. Changing `email` stages the new address in `pending_email` and sends a confirmation link to that address; the login email stays the same until POST /auth/confirm-email-change succeeds.
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
// @Failure      409   {object}  models.ErrorResponse  "Email already taken"
// @Failure      503   {object}  models.ErrorResponse  "Production email change not configured"
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
	if req.Email != "" {
		newEmail := normalizeEmailAddr(req.Email)
		if newEmail != "" && newEmail != normalizeEmailAddr(existing.Email) {
			if !h.emailChangeDeliveryReady(c) {
				return
			}
			other, ferr := h.repo.FindByEmail(c.Request.Context(), newEmail)
			if ferr == nil && other.ID != existing.ID {
				c.JSON(http.StatusConflict, models.ErrorResponse{Error: repository.ErrDuplicateEmail.Error()})
				return
			}
			update["pending_email"] = newEmail
		}
	}
	if req.Role != "" && callerRole == "admin" {
		update["role"] = req.Role
	}

	if len(update) == 0 {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "no fields to update"})
		return
	}

	var ecTok string
	var sendTo string
	if pe, ok := update["pending_email"].(string); ok && pe != "" {
		tok, terr := auth.GenerateEmailChangeToken(targetID, pe, h.cfg.JWTSecret, h.cfg.EmailChangeTokenMinutes)
		if terr != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "failed to generate email change token"})
			return
		}
		ecTok = tok
		sendTo = pe
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

	if sendTo != "" {
		link := passwordResetLink(h.cfg.EmailChangeFrontendURL, ecTok)
		if err := h.mailer.SendEmailChange(c.Request.Context(), sendTo, link); err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "failed to send email change confirmation"})
			return
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
