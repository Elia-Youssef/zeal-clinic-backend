package config

import (
	"clinic-api/internal/buildmode"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/joho/godotenv"
)

const appDataDirName = "Zeal Clinic"

type Config struct {
	Port          string
	JWTSecret     string
	JWTLifetime   time.Duration
	PeerURL       string
	SyncSecret    string
	SentryDSN     string
	PublicURL     string
	PublishSecret string
}

var current *Config

// Load parses the embedded env file (local.env.defaults or cloud.env.defaults) into Config.
func Load() *Config {
	env, err := godotenv.Unmarshal(embeddedEnv)
	if err != nil {
		log.Printf("[config] failed to parse embedded env: %v", err)
		env = map[string]string{}
	}

	cfg := &Config{
		Port:          env["PORT"],
		JWTSecret:     env["JWT_SECRET"],
		JWTLifetime:   parseDuration(env["JWT_LIFETIME"]),
		PeerURL:       env["PEER_URL"],
		SyncSecret:    env["SYNC_SECRET"],
		SentryDSN:     env["SENTRY_DSN"],
		PublicURL:     env["PUBLIC_URL"],
		PublishSecret: env["PUBLISH_SECRET"],
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

// SharedDataDir is the per-user install data dir: %LOCALAPPDATA%\Zeal Clinic\Data
// (the installer puts clinic.db here; the exe lives in the sibling App\).
func SharedDataDir() string {
	if runtime.GOOS == "windows" {
		if ad := os.Getenv("LOCALAPPDATA"); ad != "" {
			return filepath.Join(ad, appDataDirName, "Data")
		}
	}
	return ""
}

func DataDir() string {
	if buildmode.Version != "dev" {
		if dir := SharedDataDir(); dir != "" {
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				return dir
			}
		}
	}
	return "./tmp"
}

func parseDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		log.Printf("[config] Invalid JWT_LIFETIME %q, defaulting to 14h", s)
		return 14 * time.Hour
	}
	return d
}
