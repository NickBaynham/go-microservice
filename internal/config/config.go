package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

const defaultJWTSecret = "change-me-in-production"

type Config struct {
	Port           string
	TLSPort        string
	MongoURI       string
	MongoDB        string
	JWTSecret      string
	JWTExpireHours string

	// Password reset (JWT link in email when SMTP is configured; required for forgot-password in production).
	PasswordResetFrontendURL  string
	PasswordResetTokenMinutes int

	SMTPHost     string
	SMTPPort     string
	SMTPUser     string
	SMTPPassword string
	SMTPFrom     string

	// CORS — browser Origin values (scheme://host:port). See CORSAllowedOriginsFromEnv.
	CORSAllowedOrigins []string

	// TLS
	Env     string // "development", "test", "production"
	TLSCert string // path to cert PEM  (prod: set by certbot; dev: auto-generated)
	TLSKey  string // path to key PEM
}

// IsProduction reports whether the app runs in a live production profile.
// Accepts both "production" (e.g. docker-compose) and "prod" (CDK).
func (c *Config) IsProduction() bool {
	return isProductionLike(c.Env)
}

// IsTestEnv is true when ENV=test (used only for automated tests, e.g. optional reset_token in API responses).
func (c *Config) IsTestEnv() bool {
	return strings.EqualFold(strings.TrimSpace(c.Env), "test")
}

// ServeHTTPOnly is true when TLS terminates at a reverse proxy (nginx, ALB) and
// the Go process should listen for plain HTTP on Port only.
//
// Set LISTEN_HTTP=true for ECS/Fargate (scratch image) where in-container TLS is not used.
// When unset, production with no TLS_* paths also implies HTTP-only.
func (c *Config) ServeHTTPOnly() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LISTEN_HTTP"))) {
	case "1", "true", "yes":
		return true
	}
	return c.IsProduction() && (c.TLSCert == "" || c.TLSKey == "")
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, reading from environment")
	}

	env := getEnv("ENV", "development")
	jwtSecret := getEnv("JWT_SECRET", defaultJWTSecret)
	jwtExpireHours := validateJWTExpireHours(getEnv("JWT_EXPIRE_HOURS", "24"), env)

	if isProductionLike(env) {
		if jwtSecret == defaultJWTSecret || len(jwtSecret) < 32 {
			log.Fatal("JWT_SECRET must be set to a strong secret (at least 32 characters) in production")
		}
	}

	corsOrigins := CORSAllowedOriginsFromEnv(getEnv("CORS_ALLOWED_ORIGINS", ""), env)

	prMinutes := getEnvInt("PASSWORD_RESET_TOKEN_MINUTES", 60)
	if prMinutes < 1 {
		prMinutes = 1
	}
	if prMinutes > 10080 {
		prMinutes = 10080
	}
	resetFront := strings.TrimSpace(getEnv("PASSWORD_RESET_FRONTEND_URL", ""))
	if resetFront == "" && !isProductionLike(env) {
		resetFront = "http://localhost:5173/reset-password"
	}

	return &Config{
		Port:                      getEnv("PORT", "8080"),
		TLSPort:                   getEnv("TLS_PORT", "8443"),
		MongoURI:                  getEnv("MONGO_URI", "mongodb://localhost:27017"),
		MongoDB:                   getEnv("MONGO_DB", "userservice"),
		JWTSecret:                 jwtSecret,
		JWTExpireHours:            jwtExpireHours,
		PasswordResetFrontendURL:  resetFront,
		PasswordResetTokenMinutes: prMinutes,
		SMTPHost:                  getEnv("SMTP_HOST", ""),
		SMTPPort:                  getEnv("SMTP_PORT", ""),
		SMTPUser:                  getEnv("SMTP_USER", ""),
		SMTPPassword:              getEnv("SMTP_PASSWORD", ""),
		SMTPFrom:                  getEnv("SMTP_FROM", ""),
		CORSAllowedOrigins:        corsOrigins,
		Env:                       env,
		TLSCert:                   getEnv("TLS_CERT", ""),
		TLSKey:                    getEnv("TLS_KEY", ""),
	}
}

// CORSAllowedOriginsFromEnv parses a comma-separated CORS_ALLOWED_ORIGINS value.
// When raw is empty and env is not production-like, returns common local Vite preview origins.
// When raw is empty in production-like env, returns nil (no browser origins; set CORS_ALLOWED_ORIGINS for SPAs).
func CORSAllowedOriginsFromEnv(raw, env string) []string {
	raw = strings.TrimSpace(raw)
	if raw != "" {
		var out []string
		for _, p := range strings.Split(raw, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	if isProductionLike(env) {
		return nil
	}
	return []string{
		"http://localhost:5173",
		"http://127.0.0.1:5173",
		"http://localhost:4173",
		"http://127.0.0.1:4173",
	}
}

func isProductionLike(env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "production", "prod":
		return true
	default:
		return false
	}
}

func validateJWTExpireHours(raw, env string) string {
	h, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || h < 0 || h > 8760 {
		if isProductionLike(env) {
			log.Fatalf("JWT_EXPIRE_HOURS must be an integer between 0 and 8760, got %q", raw)
		}
		log.Printf("Invalid JWT_EXPIRE_HOURS %q, using default 24", raw)
		return "24"
	}
	return strconv.Itoa(h)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	s := strings.TrimSpace(os.Getenv(key))
	if s == "" {
		return fallback
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return v
}
