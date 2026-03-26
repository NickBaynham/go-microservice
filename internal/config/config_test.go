package config_test

import (
	"os"
	"testing"

	"go-microservice/internal/config"
)

func TestLoad_Defaults(t *testing.T) {
	// Ensure no env vars interfere
	unset := []string{
		"PORT", "TLS_PORT", "MONGO_URI", "MONGO_DB", "JWT_SECRET", "JWT_EXPIRE_HOURS", "JWT_ACCESS_EXPIRE_MINUTES", "JWT_REFRESH_EXPIRE_HOURS", "ENV", "TLS_CERT", "TLS_KEY",
		"PASSWORD_RESET_FRONTEND_URL", "PASSWORD_RESET_TOKEN_MINUTES",
		"SMTP_HOST", "SMTP_PORT", "SMTP_USER", "SMTP_PASSWORD", "SMTP_FROM",
		"LOG_LEVEL", "LOG_JSON", "METRICS_ENABLED",
		"EMAIL_VERIFICATION_REQUIRED", "EMAIL_VERIFICATION_FRONTEND_URL", "EMAIL_VERIFICATION_TOKEN_MINUTES",
		"EMAIL_CHANGE_FRONTEND_URL", "EMAIL_CHANGE_TOKEN_MINUTES",
		"TURNSTILE_SECRET_KEY", "LOGIN_LOCKOUT_MAX_ATTEMPTS", "LOGIN_LOCKOUT_MINUTES",
	}
	for _, k := range unset {
		os.Unsetenv(k)
	}

	cfg := config.Load()

	if cfg.EmailVerificationRequired {
		t.Error("EmailVerificationRequired: want false by default")
	}
	if cfg.TurnstileSecretKey != "" {
		t.Errorf("TurnstileSecretKey: want empty by default, got %q", cfg.TurnstileSecretKey)
	}
	if cfg.FailedLoginMaxAttempts != 5 {
		t.Errorf("FailedLoginMaxAttempts: got %d, want 5", cfg.FailedLoginMaxAttempts)
	}
	if cfg.FailedLoginLockoutMinutes != 15 {
		t.Errorf("FailedLoginLockoutMinutes: got %d, want 15", cfg.FailedLoginLockoutMinutes)
	}
	if cfg.LogJSON {
		t.Error("LogJSON: want false in development by default")
	}
	if !cfg.MetricsEnabled {
		t.Error("MetricsEnabled: want true by default")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel: got %q, want info", cfg.LogLevel)
	}

	if cfg.JWTAccessExpireMinutes != 15 {
		t.Errorf("JWTAccessExpireMinutes: got %d, want 15 (default when JWT_ACCESS_EXPIRE_MINUTES unset)", cfg.JWTAccessExpireMinutes)
	}
	if cfg.JWTRefreshExpireHours != 720 {
		t.Errorf("JWTRefreshExpireHours: got %d, want 720", cfg.JWTRefreshExpireHours)
	}

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"Port", cfg.Port, "8080"},
		{"TLSPort", cfg.TLSPort, "8443"},
		{"MongoURI", cfg.MongoURI, "mongodb://localhost:27017"},
		{"MongoDB", cfg.MongoDB, "userservice"},
		{"JWTExpireHours", cfg.JWTExpireHours, "24"},
		{"Env", cfg.Env, "development"},
		{"TLSCert", cfg.TLSCert, ""},
		{"TLSKey", cfg.TLSKey, ""},
		{"PasswordResetFrontendURL", cfg.PasswordResetFrontendURL, "http://localhost:5173/reset-password"},
		{"EmailChangeFrontendURL", cfg.EmailChangeFrontendURL, "http://localhost:5173/confirm-email-change"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s: got %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestLoad_InvalidJWTExpireHours_DevelopmentFallsBackTo24(t *testing.T) {
	unset := []string{
		"PORT", "TLS_PORT", "MONGO_URI", "MONGO_DB", "JWT_SECRET", "JWT_EXPIRE_HOURS", "JWT_ACCESS_EXPIRE_MINUTES", "JWT_REFRESH_EXPIRE_HOURS", "ENV", "TLS_CERT", "TLS_KEY",
		"PASSWORD_RESET_FRONTEND_URL", "PASSWORD_RESET_TOKEN_MINUTES",
		"SMTP_HOST", "SMTP_PORT", "SMTP_USER", "SMTP_PASSWORD", "SMTP_FROM",
		"LOG_LEVEL", "LOG_JSON", "METRICS_ENABLED",
		"EMAIL_VERIFICATION_REQUIRED", "EMAIL_VERIFICATION_FRONTEND_URL", "EMAIL_VERIFICATION_TOKEN_MINUTES",
		"EMAIL_CHANGE_FRONTEND_URL", "EMAIL_CHANGE_TOKEN_MINUTES",
		"TURNSTILE_SECRET_KEY", "LOGIN_LOCKOUT_MAX_ATTEMPTS", "LOGIN_LOCKOUT_MINUTES",
	}
	for _, k := range unset {
		os.Unsetenv(k)
	}
	os.Setenv("JWT_EXPIRE_HOURS", "not-valid")
	defer os.Unsetenv("JWT_EXPIRE_HOURS")

	cfg := config.Load()
	if cfg.JWTExpireHours != "24" {
		t.Errorf("JWTExpireHours: got %q, want 24", cfg.JWTExpireHours)
	}
	if cfg.JWTAccessExpireMinutes != 15 {
		t.Errorf("JWTAccessExpireMinutes: got %d, want 15 when JWT_EXPIRE_HOURS invalid and JWT_ACCESS unset", cfg.JWTAccessExpireMinutes)
	}
}

func TestLoad_OverridesFromEnv(t *testing.T) {
	os.Setenv("PORT", "9090")
	os.Setenv("TLS_PORT", "9443")
	os.Setenv("MONGO_URI", "mongodb://mongo:27017")
	os.Setenv("MONGO_DB", "testdb")
	os.Setenv("JWT_SECRET", "my-production-jwt-secret-at-least-32-chars!!")
	os.Setenv("JWT_EXPIRE_HOURS", "48")
	os.Setenv("ENV", "production")
	os.Setenv("TLS_CERT", "/certs/cert.pem")
	os.Setenv("TLS_KEY", "/certs/key.pem")
	defer func() {
		for _, k := range []string{
			"PORT", "TLS_PORT", "MONGO_URI", "MONGO_DB", "JWT_SECRET", "JWT_EXPIRE_HOURS", "JWT_ACCESS_EXPIRE_MINUTES", "JWT_REFRESH_EXPIRE_HOURS", "ENV", "TLS_CERT", "TLS_KEY",
			"PASSWORD_RESET_FRONTEND_URL", "PASSWORD_RESET_TOKEN_MINUTES",
			"SMTP_HOST", "SMTP_PORT", "SMTP_USER", "SMTP_PASSWORD", "SMTP_FROM",
			"LOG_LEVEL", "LOG_JSON", "METRICS_ENABLED",
			"EMAIL_VERIFICATION_REQUIRED", "EMAIL_VERIFICATION_FRONTEND_URL", "EMAIL_VERIFICATION_TOKEN_MINUTES",
			"EMAIL_CHANGE_FRONTEND_URL", "EMAIL_CHANGE_TOKEN_MINUTES",
			"TURNSTILE_SECRET_KEY", "LOGIN_LOCKOUT_MAX_ATTEMPTS", "LOGIN_LOCKOUT_MINUTES",
		} {
			os.Unsetenv(k)
		}
	}()

	cfg := config.Load()

	if !cfg.LogJSON {
		t.Error("LogJSON: want true by default in production when LOG_JSON unset")
	}
	if cfg.JWTAccessExpireMinutes != 48*60 {
		t.Errorf("JWTAccessExpireMinutes: got %d, want %d from JWT_EXPIRE_HOURS=48", cfg.JWTAccessExpireMinutes, 48*60)
	}

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"Port", cfg.Port, "9090"},
		{"TLSPort", cfg.TLSPort, "9443"},
		{"MongoURI", cfg.MongoURI, "mongodb://mongo:27017"},
		{"MongoDB", cfg.MongoDB, "testdb"},
		{"JWTSecret", cfg.JWTSecret, "my-production-jwt-secret-at-least-32-chars!!"},
		{"JWTExpireHours", cfg.JWTExpireHours, "48"},
		{"Env", cfg.Env, "production"},
		{"TLSCert", cfg.TLSCert, "/certs/cert.pem"},
		{"TLSKey", cfg.TLSKey, "/certs/key.pem"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s: got %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}
