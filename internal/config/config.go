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
	Port            string
	JWTSecret       string
	JWTLifetime     time.Duration
	DBEncryptionKey string
	PeerURL         string
	SyncSecret      string
}

var current *Config

func Load() *Config {
	_ = godotenv.Load()
	if exe, err := os.Executable(); err == nil {
		_ = godotenv.Load(filepath.Join(filepath.Dir(exe), ".env"))
	}

	cfg := &Config{
		Port:            getEnv("PORT", "55555"),
		JWTSecret:       getEnv("JWT_SECRET", "dev-only-clinic-jwt-secret-not-for-release"),
		JWTLifetime:     parseDuration(getEnv("JWT_LIFETIME", "14h")),
		DBEncryptionKey: getEnv("DB_ENCRYPTION_KEY", ""),
		PeerURL:         getEnv("PEER_URL", ""),
		SyncSecret:      getEnv("SYNC_SECRET", ""),
	}

	if cfg.DBEncryptionKey == "" {
		log.Fatal("[config] DB_ENCRYPTION_KEY is required (64 hex chars / 32 bytes)")
	}

	current = cfg
	return cfg
}

// Current returns the loaded config.
func Current() *Config {
	if current == nil {
		log.Fatal("[config] Current() called before Load()")
	}
	return current
}

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
		log.Printf("[config] Invalid JWT_LIFETIME %q, defaulting to 14h", s)
		return 14 * time.Hour
	}
	return d
}
