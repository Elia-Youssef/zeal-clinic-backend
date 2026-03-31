package config

import (
	"log"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

var (
	Port         string
	DBPath       string
	JWTSecret    string
	JWTLifetime  time.Duration
	TLSCert      string
	TLSKey       string
	CORSOrigins  []string
)

func Load() {
	_ = godotenv.Load() // silently ignore if .env doesn't exist
	Port = getEnv("PORT", "8080")
	DBPath = getEnv("DB_PATH", "clinic.db")
	JWTSecret = getEnv("JWT_SECRET", "dev-only-clinic-jwt-secret-not-for-release")
	JWTLifetime = parseDuration(getEnv("JWT_LIFETIME", "24h"))
	TLSCert = getEnv("TLS_CERT", "certs/server.crt")
	TLSKey = getEnv("TLS_KEY", "certs/server.key")

	origins := getEnv("CORS_ORIGINS", "http://localhost:3000,http://127.0.0.1:3000")
	CORSOrigins = strings.Split(origins, ",")

	if JWTSecret == "dev-only-clinic-jwt-secret-not-for-release" {
		log.Println("[config] WARNING: Using default JWT_SECRET — set JWT_SECRET env var for production")
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		log.Printf("[config] Invalid JWT_LIFETIME %q, defaulting to 24h", s)
		return 24 * time.Hour
	}
	return d
}
