package auth

import (
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// EmailChangeJWTPurpose identifies JWTs used only to confirm a pending email address change.
const EmailChangeJWTPurpose = "email_change"

// EmailChangeClaims binds a user id (subject) to the new email they must confirm.
type EmailChangeClaims struct {
	Purpose  string `json:"purpose"`
	NewEmail string `json:"new_email"`
	jwt.RegisteredClaims
}

// GenerateEmailChangeToken issues a JWT for confirming a move to newEmail (userID is Mongo hex id).
func GenerateEmailChangeToken(userID, newEmail, secret string, minutes int) (string, error) {
	userID = strings.TrimSpace(userID)
	newEmail = strings.TrimSpace(strings.ToLower(newEmail))
	if userID == "" || newEmail == "" || secret == "" {
		return "", errors.New("missing user id, new email, or secret")
	}
	if minutes < 1 {
		minutes = 1440
	}
	if minutes > 10080 {
		minutes = 10080
	}
	now := time.Now()
	claims := &EmailChangeClaims{
		Purpose:  EmailChangeJWTPurpose,
		NewEmail: newEmail,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(minutes) * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString([]byte(secret))
}

// ValidateEmailChangeToken returns the user id (hex) and normalized new email from a valid token.
func ValidateEmailChangeToken(tokenStr, secret string) (userID, newEmail string, err error) {
	token, err := jwt.ParseWithClaims(tokenStr, &EmailChangeClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return "", "", err
	}
	claims, ok := token.Claims.(*EmailChangeClaims)
	if !ok || !token.Valid {
		return "", "", errors.New("invalid token")
	}
	if claims.Purpose != EmailChangeJWTPurpose {
		return "", "", errors.New("invalid token purpose")
	}
	if claims.Subject == "" || strings.TrimSpace(claims.NewEmail) == "" {
		return "", "", errors.New("invalid token claims")
	}
	return claims.Subject, strings.TrimSpace(strings.ToLower(claims.NewEmail)), nil
}
