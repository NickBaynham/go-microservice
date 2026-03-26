package auth_test

import (
	"testing"

	"go-microservice/internal/auth"
)

func TestEmailChangeToken_RoundTrip(t *testing.T) {
	tok, err := auth.GenerateEmailChangeToken("64b1f9e2c3d4e5f6a7b8c9d0", "New@Example.com", testSecret, 60)
	if err != nil {
		t.Fatal(err)
	}
	uid, em, err := auth.ValidateEmailChangeToken(tok, testSecret)
	if err != nil {
		t.Fatal(err)
	}
	if uid != "64b1f9e2c3d4e5f6a7b8c9d0" || em != "new@example.com" {
		t.Fatalf("got uid=%q email=%q", uid, em)
	}
}

func TestValidateEmailChangeToken_WrongPurpose(t *testing.T) {
	tok, err := auth.GenerateEmailVerificationToken("64b1f9e2c3d4e5f6a7b8c9d0", testSecret, 60)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = auth.ValidateEmailChangeToken(tok, testSecret)
	if err == nil {
		t.Fatal("expected error")
	}
}
