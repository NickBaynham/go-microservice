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
	JWTExpireHours string // legacy display; access TTL prefers JWT_ACCESS_EXPIRE_MINUTES or JWT_EXPIRE_HOURS

	// Short-lived access JWT + long-lived rotating refresh (stored hashed in MongoDB).
	JWTAccessExpireMinutes int
	JWTRefreshExpireHours  int

	// Password reset (JWT link in email when SMTP is configured; required for forgot-password in production).
	PasswordResetFrontendURL  string
	PasswordResetTokenMinutes int

	// Email verification after register (opt-in via EMAIL_VERIFICATION_REQUIRED). In production, requires SMTP and EMAIL_VERIFICATION_FRONTEND_URL.
	EmailVerificationRequired     bool
	EmailVerificationFrontendURL  string
	EmailVerificationTokenMinutes int

	// Pending email change: PUT /users/:id with a new email sets pending_email and emails a confirm link. Production requires SMTP and EMAIL_CHANGE_FRONTEND_URL.
	EmailChangeFrontendURL  string
	EmailChangeTokenMinutes int

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

	// Observability — structured slog (request_id, user_id in access logs) and Prometheus /metrics
	LogLevel       string // debug | info | warn | error
	LogJSON        bool   // JSON logs (default on in production/prod unless LOG_JSON overrides)
	MetricsEnabled bool   // expose GET /metrics for scraping (default true)

	// Abuse / safety — Cloudflare Turnstile on register & login when TURNSTILE_SECRET_KEY is set.
	TurnstileSecretKey string

	// Per-account login lockout after failed password attempts (Mongo-backed). Zero max attempts disables.
	FailedLoginMaxAttempts    int
	FailedLoginLockoutMinutes int
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

	accessMin := 15
	if v, ok := getenvIntOptional("JWT_ACCESS_EXPIRE_MINUTES"); ok {
		accessMin = v
	} else if hStr := strings.TrimSpace(os.Getenv("JWT_EXPIRE_HOURS")); hStr != "" {
		if h, err := strconv.Atoi(hStr); err == nil && h >= 0 && h <= 8760 {
			accessMin = h * 60
		}
	}
	if accessMin < 1 {
		accessMin = 1
	}
	if accessMin > 10080 {
		accessMin = 10080
	}

	refreshH := getEnvInt("JWT_REFRESH_EXPIRE_HOURS", 720)
	if refreshH < 1 {
		refreshH = 1
	}
	if refreshH > 8760 {
		refreshH = 8760
	}

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

	evRequired := parseEnvBoolDefault(os.Getenv("EMAIL_VERIFICATION_REQUIRED"), false)
	evFront := strings.TrimSpace(getEnv("EMAIL_VERIFICATION_FRONTEND_URL", ""))
	if evFront == "" && !isProductionLike(env) {
		evFront = "http://localhost:5173/verify-email"
	}
	evTokMin := getEnvInt("EMAIL_VERIFICATION_TOKEN_MINUTES", 1440)
	if evTokMin < 1 {
		evTokMin = 1
	}
	if evTokMin > 10080 {
		evTokMin = 10080
	}

	ecFront := strings.TrimSpace(getEnv("EMAIL_CHANGE_FRONTEND_URL", ""))
	if ecFront == "" && !isProductionLike(env) {
		ecFront = "http://localhost:5173/confirm-email-change"
	}
	ecTokMin := getEnvInt("EMAIL_CHANGE_TOKEN_MINUTES", 1440)
	if ecTokMin < 1 {
		ecTokMin = 1
	}
	if ecTokMin > 10080 {
		ecTokMin = 10080
	}

	logJSON := isProductionLike(env)
	if v := strings.TrimSpace(os.Getenv("LOG_JSON")); v != "" {
		logJSON = parseEnvBoolDefault(v, logJSON)
	}
	logLevel := strings.TrimSpace(getEnv("LOG_LEVEL", "info"))
	metricsEnabled := parseEnvBoolDefault(os.Getenv("METRICS_ENABLED"), true)

	turnstileSecret := strings.TrimSpace(getEnv("TURNSTILE_SECRET_KEY", ""))

	lockMax := 5
	if s := strings.TrimSpace(os.Getenv("LOGIN_LOCKOUT_MAX_ATTEMPTS")); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			lockMax = v
		}
	}
	if lockMax < 0 {
		lockMax = 0
	}
	if lockMax > 100 {
		lockMax = 100
	}
	lockMin := 15
	if s := strings.TrimSpace(os.Getenv("LOGIN_LOCKOUT_MINUTES")); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			lockMin = v
		}
	}
	if lockMin < 1 {
		lockMin = 1
	}
	if lockMin > 10080 {
		lockMin = 10080
	}

	return &Config{
		Port:                      getEnv("PORT", "8080"),
		TLSPort:                   getEnv("TLS_PORT", "8443"),
		MongoURI:                  getEnv("MONGO_URI", "mongodb://localhost:27017"),
		MongoDB:                   getEnv("MONGO_DB", "userservice"),
		JWTSecret:                 jwtSecret,
		JWTExpireHours:            jwtExpireHours,
		JWTAccessExpireMinutes:    accessMin,
		JWTRefreshExpireHours:     refreshH,
		PasswordResetFrontendURL:      resetFront,
		PasswordResetTokenMinutes:     prMinutes,
		EmailVerificationRequired:     evRequired,
		EmailVerificationFrontendURL:  evFront,
		EmailVerificationTokenMinutes: evTokMin,
		EmailChangeFrontendURL:        ecFront,
		EmailChangeTokenMinutes:       ecTokMin,
		SMTPHost:                      getEnv("SMTP_HOST", ""),
		SMTPPort:                  getEnv("SMTP_PORT", ""),
		SMTPUser:                  getEnv("SMTP_USER", ""),
		SMTPPassword:              getEnv("SMTP_PASSWORD", ""),
		SMTPFrom:                  getEnv("SMTP_FROM", ""),
		CORSAllowedOrigins:        corsOrigins,
		Env:                       env,
		TLSCert:                   getEnv("TLS_CERT", ""),
		TLSKey:                    getEnv("TLS_KEY", ""),
		LogLevel:                  logLevel,
		LogJSON:                   logJSON,
		MetricsEnabled:            metricsEnabled,
		TurnstileSecretKey:        turnstileSecret,
		FailedLoginMaxAttempts:    lockMax,
		FailedLoginLockoutMinutes: lockMin,
	}
}

// TurnstileEnabled is true when server-side Turnstile verification should run on register/login.
func (c *Config) TurnstileEnabled() bool {
	return strings.TrimSpace(c.TurnstileSecretKey) != ""
}

// LoginLockoutEnabled is true when failed password attempts trigger a temporary account lockout.
func (c *Config) LoginLockoutEnabled() bool {
	return c.FailedLoginMaxAttempts > 0 && c.FailedLoginLockoutMinutes > 0
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

func getenvIntOptional(key string) (int, bool) {
	s := strings.TrimSpace(os.Getenv(key))
	if s == "" {
		return 0, false
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return v, true
}

func parseEnvBoolDefault(raw string, defaultVal bool) bool {
	s := strings.TrimSpace(strings.ToLower(raw))
	if s == "" {
		return defaultVal
	}
	switch s {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return defaultVal
	}
}
