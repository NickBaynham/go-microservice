package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port           string
	TLSPort        string
	MongoURI       string
	MongoDB        string
	JWTSecret      string
	JWTExpireHours string

	// TLS
	Env     string // "development", "test", "production"
	TLSCert string // path to cert PEM  (prod: set by certbot; dev: auto-generated)
	TLSKey  string // path to key PEM
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, reading from environment")
	}

	return &Config{
		Port:           getEnv("PORT", "8080"),
		TLSPort:        getEnv("TLS_PORT", "8443"),
		MongoURI:       getEnv("MONGO_URI", "mongodb://localhost:27017"),
		MongoDB:        getEnv("MONGO_DB", "userservice"),
		JWTSecret:      getEnv("JWT_SECRET", "change-me-in-production"),
		JWTExpireHours: getEnv("JWT_EXPIRE_HOURS", "24"),
		Env:            getEnv("ENV", "development"),
		TLSCert:        getEnv("TLS_CERT", ""),
		TLSKey:         getEnv("TLS_KEY", ""),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
