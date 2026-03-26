package mail_test

import (
	"context"
	"testing"

	"go-microservice/internal/config"
	"go-microservice/internal/mail"
)

func TestSendEmailChange_LogsWithoutSMTP(t *testing.T) {
	cfg := &config.Config{Env: "development"}
	s := mail.NewSender(cfg)
	if err := s.SendEmailChange(context.Background(), "u@example.com", "http://localhost/confirm?token=x"); err != nil {
		t.Fatal(err)
	}
}

func TestSendEmailVerification_LogsWithoutSMTP(t *testing.T) {
	cfg := &config.Config{Env: "development", SMTPHost: ""}
	s := mail.NewSender(cfg)
	if err := s.SendEmailVerification(context.Background(), "u@example.com", "http://localhost/verify?token=x"); err != nil {
		t.Fatal(err)
	}
}

func TestSendPasswordReset_LogsWithoutSMTP(t *testing.T) {
	cfg := &config.Config{
		Env:         "development",
		SMTPHost:    "",
		SMTPPort:    "",
		SMTPFrom:    "",
		SMTPUser:    "",
		SMTPPassword: "",
	}
	s := mail.NewSender(cfg)
	if err := s.SendPasswordReset(context.Background(), "u@example.com", "http://localhost/reset?token=x"); err != nil {
		t.Fatal(err)
	}
}

func TestSendPasswordReset_ProductionWithoutSMTP(t *testing.T) {
	cfg := &config.Config{
		Env:      "production",
		SMTPHost: "",
	}
	s := mail.NewSender(cfg)
	err := s.SendPasswordReset(context.Background(), "u@example.com", "http://app/reset?token=x")
	if err == nil {
		t.Fatal("expected error")
	}
}
