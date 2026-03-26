package auth_test

import (
	"testing"
	"time"

	"go-microservice/internal/auth"
)

const testSecret = "unit-test-secret"

func TestGenerateToken_ValidClaims(t *testing.T) {
	token, err := auth.GenerateToken("user123", "alice@example.com", "admin", testSecret, "24")
	if err != nil {
		t.Fatalf("GenerateToken: unexpected error: %v", err)
	}
	if token == "" {
		t.Fatal("GenerateToken: expected non-empty token")
	}
}

func TestGenerateToken_InvalidExpireHours_ReturnsError(t *testing.T) {
	_, err := auth.GenerateToken("user123", "alice@example.com", "user", testSecret, "not-a-number")
	if err == nil {
		t.Fatal("GenerateToken: expected error with invalid expiry")
	}
	// Hours are clamped when converted to minutes; invalid access TTL is only from GenerateAccessToken.
	_, err = auth.GenerateAccessToken("user123", "alice@example.com", "user", testSecret, 10081)
	if err == nil {
		t.Fatal("GenerateAccessToken: expected error when expiry minutes > 10080")
	}
}

func TestValidateToken_RoundTrip(t *testing.T) {
	token, err := auth.GenerateToken("abc123", "bob@example.com", "user", testSecret, "1")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	claims, err := auth.ValidateToken(token, testSecret)
	if err != nil {
		t.Fatalf("ValidateToken: unexpected error: %v", err)
	}

	if claims.UserID != "abc123" {
		t.Errorf("UserID: got %q, want %q", claims.UserID, "abc123")
	}
	if claims.Email != "bob@example.com" {
		t.Errorf("Email: got %q, want %q", claims.Email, "bob@example.com")
	}
	if claims.Role != "user" {
		t.Errorf("Role: got %q, want %q", claims.Role, "user")
	}
}

func TestValidateToken_WrongSecret(t *testing.T) {
	token, err := auth.GenerateToken("user1", "test@example.com", "user", testSecret, "1")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	_, err = auth.ValidateToken(token, "wrong-secret")
	if err == nil {
		t.Fatal("ValidateToken: expected error with wrong secret, got nil")
	}
}

func TestValidateToken_MalformedToken(t *testing.T) {
	_, err := auth.ValidateToken("this.is.notvalid", testSecret)
	if err == nil {
		t.Fatal("ValidateToken: expected error for malformed token, got nil")
	}
}

func TestValidateToken_EmptyToken(t *testing.T) {
	_, err := auth.ValidateToken("", testSecret)
	if err == nil {
		t.Fatal("ValidateToken: expected error for empty token, got nil")
	}
}

func TestValidateToken_ExpiredToken(t *testing.T) {
	// Generate a token that expires in 0 hours (immediately expired)
	token, err := auth.GenerateToken("user1", "test@example.com", "user", testSecret, "0")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	// Wait a moment to ensure expiry
	time.Sleep(1100 * time.Millisecond)

	_, err = auth.ValidateToken(token, testSecret)
	if err == nil {
		t.Fatal("ValidateToken: expected error for expired token, got nil")
	}
}

func TestValidateToken_TamperedPayload(t *testing.T) {
	token, err := auth.GenerateToken("user1", "test@example.com", "user", testSecret, "1")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	// Tamper with the token by appending a character to the signature
	tampered := token + "x"
	_, err = auth.ValidateToken(tampered, testSecret)
	if err == nil {
		t.Fatal("ValidateToken: expected error for tampered token, got nil")
	}
}
