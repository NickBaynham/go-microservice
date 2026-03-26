package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// PasswordResetJWTPurpose is the JWT "purpose" claim for password-reset tokens (not for API access).
const PasswordResetJWTPurpose = "password_reset"

// PasswordResetClaims is a short-lived JWT used only for password reset (not API access).
type PasswordResetClaims struct {
	Purpose string `json:"purpose"`
	jwt.RegisteredClaims
}

// GeneratePasswordResetToken issues a signed JWT bound to userID. minutes is token lifetime (clamped).
func GeneratePasswordResetToken(userID, secret string, minutes int) (string, error) {
	if userID == "" || secret == "" {
		return "", errors.New("missing user id or secret")
	}
	if minutes < 1 {
		minutes = 60
	}
	if minutes > 10080 {
		minutes = 10080
	}
	now := time.Now()
	claims := &PasswordResetClaims{
		Purpose: PasswordResetJWTPurpose,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(minutes) * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString([]byte(secret))
}

// ValidatePasswordResetToken parses and validates a reset JWT; returns the Mongo user id (hex).
func ValidatePasswordResetToken(tokenStr, secret string) (userID string, err error) {
	token, err := jwt.ParseWithClaims(tokenStr, &PasswordResetClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return "", err
	}
	claims, ok := token.Claims.(*PasswordResetClaims)
	if !ok || !token.Valid {
		return "", errors.New("invalid token")
	}
	if claims.Purpose != PasswordResetJWTPurpose {
		return "", errors.New("invalid token purpose")
	}
	if claims.Subject == "" {
		return "", errors.New("invalid token subject")
	}
	return claims.Subject, nil
}
