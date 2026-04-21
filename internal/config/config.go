package config

import (
	"log"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port        string
	DBPath      string
	JWTSecret   string
	JWTLifetime time.Duration
	CORSOrigins []string
}

var current *Config

func Load() *Config {
	_ = godotenv.Load() // silently ignore if .env doesn't exist

	cfg := &Config{
		Port:        getEnv("PORT", "8080"),
		DBPath:      getEnv("DB_PATH", "clinic.db"),
		JWTSecret:   getEnv("JWT_SECRET", "dev-only-clinic-jwt-secret-not-for-release"),
		JWTLifetime: parseDuration(getEnv("JWT_LIFETIME", "24h")),
		CORSOrigins: strings.Split(getEnv("CORS_ORIGINS", "http://localhost:3000,http://127.0.0.1:3000"), ","),
	}

	if cfg.JWTSecret == "dev-only-clinic-jwt-secret-not-for-release" {
		log.Println("[config] WARNING: Using default JWT_SECRET — set JWT_SECRET env var for production")
	}

	current = cfg
	return cfg
}

// Current returns the last loaded config. Panics if Load hasn't been called.
func Current() *Config {
	if current == nil {
		log.Fatal("[config] Current() called before Load()")
	}
	return current
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
