package auth_test

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go-microservice/internal/auth"
)

func TestGeneratePasswordResetToken_AndValidate(t *testing.T) {
	const secret = "test-secret-at-least-32-characters-long"
	uid := "64b1f9e2c3d4e5f6a7b8c9d0"

	tok, err := auth.GeneratePasswordResetToken(uid, secret, 60)
	if err != nil {
		t.Fatal(err)
	}
	if tok == "" {
		t.Fatal("empty token")
	}
	got, err := auth.ValidatePasswordResetToken(tok, secret)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if got != uid {
		t.Fatalf("subject: got %q want %q", got, uid)
	}
}

func TestValidatePasswordResetToken_WrongSecret(t *testing.T) {
	const secret = "test-secret-at-least-32-characters-long"
	tok, err := auth.GeneratePasswordResetToken("64b1f9e2c3d4e5f6a7b8c9d0", secret, 60)
	if err != nil {
		t.Fatal(err)
	}
	_, err = auth.ValidatePasswordResetToken(tok, "other-secret-at-least-32-chars!!")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidatePasswordResetToken_LoginJWTRejected(t *testing.T) {
	const secret = "test-secret-at-least-32-characters-long"
	loginTok, err := auth.GenerateToken("64b1f9e2c3d4e5f6a7b8c9d0", "a@b.com", "user", secret, "1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = auth.ValidatePasswordResetToken(loginTok, secret)
	if err == nil {
		t.Fatal("login JWT must not validate as reset token")
	}
}

func TestValidatePasswordResetToken_Expired(t *testing.T) {
	const secret = "test-secret-at-least-32-characters-long"
	claims := &auth.PasswordResetClaims{
		Purpose: auth.PasswordResetJWTPurpose,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "64b1f9e2c3d4e5f6a7b8c9d0",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	_, err = auth.ValidatePasswordResetToken(s, secret)
	if err == nil {
		t.Fatal("expected expiry error")
	}
}

func TestGeneratePasswordResetToken_MissingInputs(t *testing.T) {
	_, err := auth.GeneratePasswordResetToken("", "secret", 60)
	if err == nil {
		t.Fatal("expected error")
	}
	_, err = auth.GeneratePasswordResetToken("id", "", 60)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidatePasswordResetToken_Garbage(t *testing.T) {
	_, err := auth.ValidatePasswordResetToken("not.a.jwt", "secret")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "token") {
		t.Fatalf("expected token error, got %v", err)
	}
}
