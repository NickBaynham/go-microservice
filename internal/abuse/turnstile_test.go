package abuse_test

import (
	"context"
	"testing"

	"go-microservice/internal/abuse"
)

func TestVerifyTurnstile_MissingSecret(t *testing.T) {
	_, err := abuse.VerifyTurnstile(context.Background(), "", "response", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestVerifyTurnstile_MissingResponse(t *testing.T) {
	_, err := abuse.VerifyTurnstile(context.Background(), "secret", "", "")
	if err == nil {
		t.Fatal("expected error")
	}
}
