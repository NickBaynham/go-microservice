package mail

import (
	"context"
	"fmt"
	"log"
	"net/smtp"
	"strings"

	"go-microservice/internal/config"
)

// Sender delivers password-reset emails when SMTP is configured; otherwise logs the link in non-production.
type Sender struct {
	cfg *config.Config
}

func NewSender(cfg *config.Config) *Sender {
	return &Sender{cfg: cfg}
}

// SendPasswordReset emails a single-use reset link. Without SMTP in development/test, logs the URL and returns nil.
func (s *Sender) SendPasswordReset(_ context.Context, toEmail, resetURL string) error {
	if strings.TrimSpace(s.cfg.SMTPHost) == "" {
		if s.cfg.IsProduction() {
			return fmt.Errorf("SMTP is not configured")
		}
		log.Printf("password reset link for %s: %s", toEmail, resetURL)
		return nil
	}

	from := strings.TrimSpace(s.cfg.SMTPFrom)
	if from == "" {
		return fmt.Errorf("SMTP_FROM is required when SMTP_HOST is set")
	}

	host := strings.TrimSpace(s.cfg.SMTPHost)
	port := strings.TrimSpace(s.cfg.SMTPPort)
	if port == "" {
		port = "587"
	}
	addr := host + ":" + port

	subject := "Password reset"
	body := "Reset your password by visiting:\n\n" + resetURL + "\n\nIf you did not request this, you can ignore this email.\n"

	msg := []byte("To: " + toEmail + "\r\n" +
		"From: " + from + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"\r\n" +
		body + "\r\n")

	user := strings.TrimSpace(s.cfg.SMTPUser)
	pass := s.cfg.SMTPPassword

	var auth smtp.Auth
	if user != "" {
		auth = smtp.PlainAuth("", user, pass, host)
	}

	return smtp.SendMail(addr, auth, from, []string{toEmail}, msg)
}

// SendEmailVerification emails a link to confirm the address. Same SMTP rules as SendPasswordReset.
func (s *Sender) SendEmailVerification(_ context.Context, toEmail, verifyURL string) error {
	if strings.TrimSpace(s.cfg.SMTPHost) == "" {
		if s.cfg.IsProduction() {
			return fmt.Errorf("SMTP is not configured")
		}
		log.Printf("email verification link for %s: %s", toEmail, verifyURL)
		return nil
	}

	from := strings.TrimSpace(s.cfg.SMTPFrom)
	if from == "" {
		return fmt.Errorf("SMTP_FROM is required when SMTP_HOST is set")
	}

	host := strings.TrimSpace(s.cfg.SMTPHost)
	port := strings.TrimSpace(s.cfg.SMTPPort)
	if port == "" {
		port = "587"
	}
	addr := host + ":" + port

	subject := "Verify your email"
	body := "Confirm your email address by visiting:\n\n" + verifyURL + "\n\nIf you did not create an account, you can ignore this email.\n"

	msg := []byte("To: " + toEmail + "\r\n" +
		"From: " + from + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"\r\n" +
		body + "\r\n")

	user := strings.TrimSpace(s.cfg.SMTPUser)
	pass := strings.TrimSpace(s.cfg.SMTPPassword)

	var auth smtp.Auth
	if user != "" {
		auth = smtp.PlainAuth("", user, pass, host)
	}

	return smtp.SendMail(addr, auth, from, []string{toEmail}, msg)
}
