package config

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/joho/godotenv"
)

const appDataDirName = "Zeal Clinic"

type Config struct {
	Port        string
	JWTSecret   string
	JWTLifetime time.Duration
}

var current *Config

func Load() *Config {
	// Load .env from cwd first (dev), then from the directory containing the
	// executable (installed layout; the installer drops a generated .env there).
	// godotenv doesn't overwrite existing env vars, so cwd wins on conflicts.
	_ = godotenv.Load()
	if exe, err := os.Executable(); err == nil {
		_ = godotenv.Load(filepath.Join(filepath.Dir(exe), ".env"))
	}

	cfg := &Config{
		Port:        getEnv("PORT", "8080"),
		JWTSecret:   getEnv("JWT_SECRET", "dev-only-clinic-jwt-secret-not-for-release"),
		JWTLifetime: parseDuration(getEnv("JWT_LIFETIME", "14h")),
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

// SharedDataDir returns the platform data directory for this app (e.g.
// %PROGRAMDATA%\Zeal Clinic on Windows), or "" if unavailable.
func SharedDataDir() string {
	if runtime.GOOS == "windows" {
		if pd := os.Getenv("PROGRAMDATA"); pd != "" {
			return filepath.Join(pd, appDataDirName)
		}
	}
	return ""
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
