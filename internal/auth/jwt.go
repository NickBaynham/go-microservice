package auth

import (
	"errors"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// GenerateAccessToken issues a short-lived API access JWT (expireMinutes 0 = already expired, for tests).
func GenerateAccessToken(userID, email, role, secret string, expireMinutes int) (string, error) {
	if expireMinutes < 0 || expireMinutes > 10080 {
		return "", errors.New("invalid access token expiry minutes")
	}
	var exp time.Time
	if expireMinutes == 0 {
		exp = time.Now()
	} else {
		exp = time.Now().Add(time.Duration(expireMinutes) * time.Minute)
	}

	claims := &Claims{
		UserID: userID,
		Email:  email,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// GenerateToken issues an access JWT from a whole-hour lifetime (backward compatible with tests and older env).
func GenerateToken(userID, email, role, secret, expireHours string) (string, error) {
	hours, err := strconv.Atoi(expireHours)
	if err != nil || hours < 0 || hours > 8760 {
		return "", errors.New("invalid JWT expiry hours")
	}
	minutes := hours * 60
	if minutes > 10080 {
		minutes = 10080
	}
	return GenerateAccessToken(userID, email, role, secret, minutes)
}

func ValidateToken(tokenStr, secret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
