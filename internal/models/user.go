package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// User represents a user in the system
// @Description User account information
type User struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"  example:"64b1f9e2c3d4e5f6a7b8c9d0"`
	Name      string             `bson:"name"          json:"name"          example:"Alice Smith"`
	Email     string             `bson:"email"         json:"email"         example:"alice@example.com"`
	Password  string             `bson:"password"      json:"-"`
	Role      string             `bson:"role"          json:"role"          example:"user"`
	CreatedAt time.Time          `bson:"created_at"    json:"created_at"    example:"2024-01-01T00:00:00Z"`
	UpdatedAt time.Time          `bson:"updated_at"    json:"updated_at"    example:"2024-01-01T00:00:00Z"`
}

// CreateUserRequest is the payload for registering a new user
// @Description Registration request body
type CreateUserRequest struct {
	Name     string `json:"name"     binding:"required,min=2" example:"Alice Smith"`
	Email    string `json:"email"    binding:"required,email" example:"alice@example.com"`
	Password string `json:"password" binding:"required,min=8" example:"securepassword"`
}

// UpdateUserRequest is the payload for updating a user
// @Description Update request body (all fields optional)
type UpdateUserRequest struct {
	Name  string `json:"name"  binding:"omitempty,min=2" example:"Alice Updated"`
	Email string `json:"email" binding:"omitempty,email" example:"newemail@example.com"`
	Role  string `json:"role"                             example:"admin"`
}

// LoginRequest is the payload for authenticating a user
// @Description Login request body
type LoginRequest struct {
	Email    string `json:"email"    binding:"required,email" example:"alice@example.com"`
	Password string `json:"password" binding:"required"        example:"securepassword"`
}

// LoginResponse is returned on successful login
// @Description Successful login response containing JWT and user info
type LoginResponse struct {
	Token string `json:"token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
	User  User   `json:"user"`
}

// ErrorResponse is a generic error response
// @Description Error response
type ErrorResponse struct {
	Error string `json:"error" example:"invalid credentials"`
}

// MessageResponse is a generic message response
// @Description Success message response
type MessageResponse struct {
	Message string `json:"message" example:"user deleted successfully"`
}

// HealthResponse is returned by the health check endpoint
// @Description Health check response
type HealthResponse struct {
	Status    string            `json:"status"    example:"healthy"`
	Timestamp time.Time         `json:"timestamp" example:"2024-01-01T00:00:00Z"`
	Checks    map[string]string `json:"checks"`
}

// ListUsersResponse wraps a list of users with a count
// @Description List of users response
type ListUsersResponse struct {
	Users []User `json:"users"`
	Count int    `json:"count" example:"5"`
}

// RegisterResponse wraps the created user with a message
// @Description Successful registration response
type RegisterResponse struct {
	Message string `json:"message" example:"user registered successfully"`
	User    User   `json:"user"`
}

// ForgotPasswordRequest asks for a reset email for the given address.
// @Description Forgot-password request
type ForgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email" example:"alice@example.com"`
}

// ForgotPasswordResponse acknowledges the request (same shape whether or not the email exists).
// @Description Forgot-password response
type ForgotPasswordResponse struct {
	Message string `json:"message" example:"If an account exists for this email, you will receive reset instructions."`
	// ResetToken is returned only when ENV=test (for automated E2E); never in production or development.
	ResetToken *string `json:"reset_token,omitempty" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
}

// ResetPasswordRequest completes a reset using the token from the email link.
// @Description Reset-password request
type ResetPasswordRequest struct {
	Token    string `json:"token"    binding:"required" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
	Password string `json:"password" binding:"required,min=8" example:"newSecurePass1"`
}
