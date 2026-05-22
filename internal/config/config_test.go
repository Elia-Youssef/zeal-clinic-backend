package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// withCleanEnv saves and clears the env keys we touch, then restores them.
// DB_ENCRYPTION_KEY is reset to a dummy value rather than unset, since Load()
// log.Fatals when it's empty and these tests aren't exercising that branch.
func withCleanEnv(t *testing.T) {
	t.Helper()
	keys := []string{"PORT", "JWT_SECRET", "JWT_LIFETIME", "DB_ENCRYPTION_KEY"}
	saved := make(map[string]string, len(keys))
	had := make(map[string]bool, len(keys))
	for _, k := range keys {
		saved[k], had[k] = os.LookupEnv(k)
		os.Unsetenv(k)
	}
	os.Setenv("DB_ENCRYPTION_KEY", "00000000000000000000000000000000000000000000000000000000000000ff")
	t.Cleanup(func() {
		for _, k := range keys {
			if had[k] {
				os.Setenv(k, saved[k])
			} else {
				os.Unsetenv(k)
			}
		}
	})
}

// inEmptyDir cd's into a fresh temp dir for the test (and restores cwd) so the
// godotenv .env loader has nothing to read from cwd.
func inEmptyDir(t *testing.T) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })
}

func TestLoad_DefaultsWhenNoEnv(t *testing.T) {
	withCleanEnv(t)
	inEmptyDir(t)

	cfg := Load()
	if cfg == nil {
		t.Fatal("Load returned nil")
	}
	if cfg.Port != "55555" {
		t.Errorf("default Port = %q want 55555", cfg.Port)
	}
	// The default JWT secret is a baked-in fallback; we only care that one exists.
	if cfg.JWTSecret == "" {
		t.Error("default JWTSecret should be non-empty")
	}
	if cfg.JWTLifetime != 14*time.Hour {
		t.Errorf("default JWTLifetime = %v want 14h", cfg.JWTLifetime)
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	withCleanEnv(t)
	inEmptyDir(t)
	os.Setenv("PORT", "9999")
	os.Setenv("JWT_SECRET", "real-secret")
	os.Setenv("JWT_LIFETIME", "2h30m")

	cfg := Load()
	if cfg.Port != "9999" || cfg.JWTSecret != "real-secret" || cfg.JWTLifetime != 2*time.Hour+30*time.Minute {
		t.Errorf("env overrides not applied: %+v", cfg)
	}
}

// A bad JWT_LIFETIME falls back to 14h rather than failing to start.
func TestLoad_BadJWTLifetimeFallsBack(t *testing.T) {
	withCleanEnv(t)
	inEmptyDir(t)
	os.Setenv("JWT_LIFETIME", "not-a-duration")

	if got := Load().JWTLifetime; got != 14*time.Hour {
		t.Errorf("expected 14h fallback, got %v", got)
	}
}

// .env from cwd is loaded, but an explicit process env var wins over it.
func TestLoad_DotEnvLoadedButOSEnvWins(t *testing.T) {
	withCleanEnv(t)
	inEmptyDir(t)
	dir, _ := os.Getwd()
	if err := os.WriteFile(filepath.Join(dir, ".env"),
		[]byte("PORT=7000\nJWT_SECRET=from-dotenv\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	os.Setenv("PORT", "8001") // explicit env beats .env

	cfg := Load()
	if cfg.Port != "8001" {
		t.Errorf("explicit env should win, got Port=%q", cfg.Port)
	}
	if cfg.JWTSecret != "from-dotenv" {
		t.Errorf("dotenv value should apply when no env override, got %q", cfg.JWTSecret)
	}
}
