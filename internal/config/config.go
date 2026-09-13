package config

import (
	"clinic-api/internal/buildmode"
	"clinic-api/internal/config/envfile"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const appDataDirName = "Zeal Clinic"

type Config struct {
	Port            string
	JWTSecret       string
	JWTLifetime     time.Duration
	PeerURL         string
	SyncSecret      string
	PublicURL       string
	PublishSecret   string
	DBEncryptionKey string
	ClinicTimezone  string
}

var current *Config

// Load reads the build's embedded env files into Config: the committed dev
// defaults (local.env.defaults or cloud.env.defaults), with the non-empty
// values of the local override (local.env or cloud.env, git-ignored) applied
// key by key when that file existed at build time.
func Load() *Config {
	env, overridden, err := envfile.Load(envFiles, envFile)
	if err != nil {
		log.Printf("[config] %v", err)
		env = map[string]string{}
	}
	if len(overridden) > 0 {
		log.Printf("[config] %s sets %s", envFile, strings.Join(overridden, ", "))
	} else {
		log.Printf("[config] no %s: running on the dev defaults", envFile)
	}
	cfg := fromEnv(env)
	current = cfg
	return cfg
}

func fromEnv(env map[string]string) *Config {
	tz := env[envfile.KeyClinicTimezone]
	if tz == "" {
		tz = "Asia/Beirut"
	}
	return &Config{
		Port:            env[envfile.KeyPort],
		JWTSecret:       env[envfile.KeyJWTSecret],
		JWTLifetime:     parseDuration(env[envfile.KeyJWTLifetime]),
		PeerURL:         env[envfile.KeyPeerURL],
		SyncSecret:      env[envfile.KeySyncSecret],
		PublicURL:       env[envfile.KeyPublicURL],
		PublishSecret:   env[envfile.KeyPublishSecret],
		DBEncryptionKey: env[envfile.KeyDBEncryptionKey],
		ClinicTimezone:  tz,
	}
}

// Current returns the loaded config.
func Current() *Config {
	if current == nil {
		log.Fatal("[config] Current() called before Load()")
	}
	return current
}

// Check validates the config for this build: every build needs a DB key of 64
// hex characters and a valid IANA clinic timezone, and a release build (see
// buildmode.Release) also refuses the committed dev values and a JWT secret
// shorter than 32 characters. The error names keys, never values.
func (c *Config) Check(release bool) error {
	if _, err := time.LoadLocation(c.ClinicTimezone); err != nil {
		return fmt.Errorf("%s: invalid timezone %q: %w", envfile.KeyClinicTimezone, c.ClinicTimezone, err)
	}
	defaults, err := envfile.Read(envFiles, envFile+envfile.DefaultsSuffix)
	if err != nil {
		return err
	}
	if p := envfile.Problems(c.secrets(), defaults, release); len(p) > 0 {
		kind := "dev"
		if release {
			kind = "release"
		}
		return fmt.Errorf("config of this %s build (%s): %s", kind, envFile, strings.Join(p, "; "))
	}
	return nil
}

// DevKeys lists the secret keys that hold a dev value: a committed default,
// or any value marked dev-only.
func (c *Config) DevKeys() []string {
	defaults, err := envfile.Read(envFiles, envFile+envfile.DefaultsSuffix)
	if err != nil {
		return nil
	}
	return envfile.DevKeys(c.secrets(), defaults)
}

func (c *Config) secrets() map[string]string {
	return map[string]string{
		envfile.KeyJWTSecret:       c.JWTSecret,
		envfile.KeySyncSecret:      c.SyncSecret,
		envfile.KeyPublishSecret:   c.PublishSecret,
		envfile.KeyDBEncryptionKey: c.DBEncryptionKey,
	}
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
	if buildmode.Cloud {
		return "./data"
	}
	return "./tmp"
}

func BackupDir() string {
	return filepath.Join(filepath.Dir(DataDir()), "backup")
}

func parseDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		log.Printf("[config] Invalid JWT_LIFETIME %q, defaulting to 14h", s)
		return 14 * time.Hour
	}
	return d
}
