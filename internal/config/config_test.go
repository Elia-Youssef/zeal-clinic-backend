package config

import (
	"os"
	"path/filepath"
	"runtime"
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
	if cfg.JWTSecret != "dev-only-clinic-jwt-secret-not-for-release" {
		t.Errorf("default JWTSecret = %q want dev-only-clinic-jwt-secret-not-for-release", cfg.JWTSecret)
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
	if cfg.Port != "9999" {
		t.Errorf("Port = %q", cfg.Port)
	}
	if cfg.JWTSecret != "real-secret" {
		t.Errorf("JWTSecret = %q", cfg.JWTSecret)
	}
	if cfg.JWTLifetime != 2*time.Hour+30*time.Minute {
		t.Errorf("JWTLifetime = %v", cfg.JWTLifetime)
	}
}

func TestLoad_BadJWTLifetimeDefaultsTo14h(t *testing.T) {
	withCleanEnv(t)
	inEmptyDir(t)
	os.Setenv("JWT_LIFETIME", "not-a-duration")

	cfg := Load()
	if cfg.JWTLifetime != 14*time.Hour {
		t.Errorf("expected 14h fallback, got %v", cfg.JWTLifetime)
	}
}

func TestLoad_PicksUpDotEnvFromCwd(t *testing.T) {
	withCleanEnv(t)
	inEmptyDir(t)
	dir, _ := os.Getwd()
	envPath := filepath.Join(dir, ".env")
	contents := "PORT=7000\nJWT_SECRET=from-dotenv\nJWT_LIFETIME=1h\n"
	if err := os.WriteFile(envPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := Load()
	if cfg.Port != "7000" {
		t.Errorf("Port = %q", cfg.Port)
	}
	if cfg.JWTSecret != "from-dotenv" {
		t.Errorf("JWTSecret = %q", cfg.JWTSecret)
	}
	if cfg.JWTLifetime != time.Hour {
		t.Errorf("JWTLifetime = %v", cfg.JWTLifetime)
	}
}

func TestLoad_OSEnvWinsOverDotEnv(t *testing.T) {
	withCleanEnv(t)
	inEmptyDir(t)
	dir, _ := os.Getwd()
	if err := os.WriteFile(filepath.Join(dir, ".env"),
		[]byte("PORT=7000\nJWT_SECRET=from-dotenv\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// godotenv.Load() doesn't overwrite existing env vars, so explicit os env wins.
	os.Setenv("PORT", "8001")

	cfg := Load()
	if cfg.Port != "8001" {
		t.Errorf("expected explicit env to win, got Port=%q", cfg.Port)
	}
	if cfg.JWTSecret != "from-dotenv" {
		t.Errorf("dotenv JWT_SECRET should still apply, got %q", cfg.JWTSecret)
	}
}

func TestCurrent_AfterLoad(t *testing.T) {
	withCleanEnv(t)
	inEmptyDir(t)
	cfg := Load()
	got := Current()
	if got != cfg {
		t.Errorf("Current() should return the same pointer as Load() (got %p want %p)", got, cfg)
	}
}

func TestSharedDataDir(t *testing.T) {
	got := SharedDataDir()
	if runtime.GOOS != "windows" {
		if got != "" {
			t.Errorf("non-windows should return empty, got %q", got)
		}
		return
	}
	// On Windows, mirror the function's logic with the current PROGRAMDATA.
	pd := os.Getenv("PROGRAMDATA")
	if pd == "" {
		if got != "" {
			t.Errorf("no PROGRAMDATA set but got %q", got)
		}
		return
	}
	want := filepath.Join(pd, appDataDirName)
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestGetEnv_FallbackUsedWhenUnset(t *testing.T) {
	os.Unsetenv("ZEAL_TEST_KEY_UNSET")
	if got := getEnv("ZEAL_TEST_KEY_UNSET", "fb"); got != "fb" {
		t.Errorf("got %q want fb", got)
	}
}

func TestGetEnv_FallbackUsedWhenEmpty(t *testing.T) {
	os.Setenv("ZEAL_TEST_KEY_EMPTY", "")
	t.Cleanup(func() { os.Unsetenv("ZEAL_TEST_KEY_EMPTY") })
	if got := getEnv("ZEAL_TEST_KEY_EMPTY", "fb"); got != "fb" {
		t.Errorf("empty env should yield fallback, got %q", got)
	}
}

func TestGetEnv_ValueWins(t *testing.T) {
	os.Setenv("ZEAL_TEST_KEY_SET", "real")
	t.Cleanup(func() { os.Unsetenv("ZEAL_TEST_KEY_SET") })
	if got := getEnv("ZEAL_TEST_KEY_SET", "fb"); got != "real" {
		t.Errorf("got %q", got)
	}
}

func TestParseDuration_ValidStrings(t *testing.T) {
	cases := map[string]time.Duration{
		"1ns":   time.Nanosecond,
		"500ms": 500 * time.Millisecond,
		"1s":    time.Second,
		"5m":    5 * time.Minute,
		"14h":   14 * time.Hour,
		"1h30m": 90 * time.Minute,
		"2h45m": 2*time.Hour + 45*time.Minute,
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			if got := parseDuration(in); got != want {
				t.Errorf("parseDuration(%q) = %v want %v", in, got, want)
			}
		})
	}
}

func TestParseDuration_InvalidDefaultsTo14h(t *testing.T) {
	cases := []string{"", "abc", "1day", "1.5", "h"}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			if got := parseDuration(in); got != 14*time.Hour {
				t.Errorf("parseDuration(%q) should default to 14h, got %v", in, got)
			}
		})
	}
}
