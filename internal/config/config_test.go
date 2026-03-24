package config_test

import (
	"os"
	"testing"

	"go-microservice/internal/config"
)

func TestLoad_Defaults(t *testing.T) {
	// Ensure no env vars interfere
	unset := []string{"PORT", "TLS_PORT", "MONGO_URI", "MONGO_DB", "JWT_SECRET", "JWT_EXPIRE_HOURS", "ENV", "TLS_CERT", "TLS_KEY"}
	for _, k := range unset {
		os.Unsetenv(k)
	}

	cfg := config.Load()

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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s: got %q, want %q", tt.name, tt.got, tt.want)
			}
		})
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
		for _, k := range []string{"PORT", "TLS_PORT", "MONGO_URI", "MONGO_DB", "JWT_SECRET", "JWT_EXPIRE_HOURS", "ENV", "TLS_CERT", "TLS_KEY"} {
			os.Unsetenv(k)
		}
	}()

	cfg := config.Load()

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
