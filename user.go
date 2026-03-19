package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type User struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"      json:"id,omitempty"`
	Name      string             `bson:"name"               json:"name"`
	Email     string             `bson:"email"              json:"email"`
	Password  string             `bson:"password"           json:"-"`
	Role      string             `bson:"role"               json:"role"`
	CreatedAt time.Time          `bson:"created_at"         json:"created_at"`
	UpdatedAt time.Time          `bson:"updated_at"         json:"updated_at"`
}

type CreateUserRequest struct {
	Name     string `json:"name"     binding:"required,min=2"`
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
	Role     string `json:"role"`
}

type UpdateUserRequest struct {
	Name  string `json:"name"  binding:"omitempty,min=2"`
	Email string `json:"email" binding:"omitempty,email"`
	Role  string `json:"role"`
}

type LoginRequest struct {
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type LoginResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}
