package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// EmailVerifyJWTPurpose is the JWT "purpose" claim for email verification (not API access).
const EmailVerifyJWTPurpose = "email_verify"

// EmailVerifyClaims is a short-lived JWT used only to confirm ownership of the email address.
type EmailVerifyClaims struct {
	Purpose string `json:"purpose"`
	jwt.RegisteredClaims
}

// GenerateEmailVerificationToken issues a signed JWT bound to userID (Mongo hex id).
func GenerateEmailVerificationToken(userID, secret string, minutes int) (string, error) {
	if userID == "" || secret == "" {
		return "", errors.New("missing user id or secret")
	}
	if minutes < 1 {
		minutes = 1440
	}
	if minutes > 10080 {
		minutes = 10080
	}
	now := time.Now()
	claims := &EmailVerifyClaims{
		Purpose: EmailVerifyJWTPurpose,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(minutes) * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString([]byte(secret))
}

// ValidateEmailVerificationToken parses and validates a verification JWT; returns the Mongo user id (hex).
func ValidateEmailVerificationToken(tokenStr, secret string) (userID string, err error) {
	token, err := jwt.ParseWithClaims(tokenStr, &EmailVerifyClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return "", err
	}
	claims, ok := token.Claims.(*EmailVerifyClaims)
	if !ok || !token.Valid {
		return "", errors.New("invalid token")
	}
	if claims.Purpose != EmailVerifyJWTPurpose {
		return "", errors.New("invalid token purpose")
	}
	if claims.Subject == "" {
		return "", errors.New("invalid token subject")
	}
	return claims.Subject, nil
}
