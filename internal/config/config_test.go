package config

import (
	"testing"
	"time"
)

func TestLoad_FromEmbeddedEnv(t *testing.T) {
	cfg := Load()
	if cfg == nil {
		t.Fatal("Load returned nil")
	}
	if cfg.Port != "55555" {
		t.Errorf("embedded Port = %q want 55555", cfg.Port)
	}
	if cfg.JWTLifetime != 14*time.Hour {
		t.Errorf("JWTLifetime = %v want 14h", cfg.JWTLifetime)
	}
}

func TestParseDuration(t *testing.T) {
	if got := parseDuration("2h30m"); got != 2*time.Hour+30*time.Minute {
		t.Errorf("parseDuration(2h30m) = %v", got)
	}
	if got := parseDuration("not-a-duration"); got != 14*time.Hour {
		t.Errorf("parseDuration(bad) = %v want 14h fallback", got)
	}
}
