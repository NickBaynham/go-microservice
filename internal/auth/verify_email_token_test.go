package auth_test

import (
	"testing"

	"go-microservice/internal/auth"
)

func TestGenerateEmailVerificationToken_RoundTrip(t *testing.T) {
	tok, err := auth.GenerateEmailVerificationToken("64b1f9e2c3d4e5f6a7b8c9d0", testSecret, 60)
	if err != nil {
		t.Fatal(err)
	}
	id, err := auth.ValidateEmailVerificationToken(tok, testSecret)
	if err != nil {
		t.Fatal(err)
	}
	if id != "64b1f9e2c3d4e5f6a7b8c9d0" {
		t.Errorf("subject: got %q", id)
	}
}

func TestValidateEmailVerificationToken_WrongPurpose(t *testing.T) {
	resetTok, err := auth.GeneratePasswordResetToken("64b1f9e2c3d4e5f6a7b8c9d0", testSecret, 60)
	if err != nil {
		t.Fatal(err)
	}
	_, err = auth.ValidateEmailVerificationToken(resetTok, testSecret)
	if err == nil {
		t.Fatal("expected error for password_reset token")
	}
}
