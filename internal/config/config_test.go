package config

import (
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"clinic-api/internal/config/envfile"
)

func TestLoad_FromEmbeddedEnv(t *testing.T) {
	cfg := Load()
	if cfg == nil {
		t.Fatal("Load returned nil")
	}
	if cfg.Port != embeddedPort {
		t.Errorf("embedded Port = %q want %s", cfg.Port, embeddedPort)
	}
	if cfg.JWTLifetime != 14*time.Hour {
		t.Errorf("JWTLifetime = %v want 14h", cfg.JWTLifetime)
	}
	// Every build, with or without an override, gets a usable DB key.
	if err := cfg.Check(false); err != nil {
		t.Errorf("Check(dev build): %v", err)
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

// The embedded defaults run a dev build, and a release build refuses each of
// their secrets by key, without quoting a value.
func TestCheck_ReleaseRefusesTheEmbeddedDefaults(t *testing.T) {
	defaults, err := envfile.Read(envFiles, envFile+envfile.DefaultsSuffix)
	if err != nil {
		t.Fatal(err)
	}
	cfg := fromEnv(defaults)
	if err := cfg.Check(false); err != nil {
		t.Errorf("a dev build refused the defaults: %v", err)
	}
	err = cfg.Check(true)
	if err == nil {
		t.Fatal("a release build accepted the dev defaults")
	}
	var present []string
	for _, k := range envfile.SecretKeys {
		if defaults[k] == "" {
			continue
		}
		present = append(present, k)
		if !strings.Contains(err.Error(), k) {
			t.Errorf("the refusal does not name %s: %v", k, err)
		}
	}
	for k, v := range defaults {
		if len(v) >= 8 && strings.Contains(err.Error(), v) {
			t.Errorf("the refusal quotes the value of %s", k)
		}
	}
	if keys := cfg.DevKeys(); !reflect.DeepEqual(keys, present) {
		t.Errorf("DevKeys = %v, want %v", keys, present)
	}

	// Release values from an override pass.
	cfg.JWTSecret = strings.Repeat("j", envfile.MinJWTSecretLen)
	cfg.SyncSecret = strings.Repeat("s", 40)
	cfg.PublishSecret = strings.Repeat("p", 40)
	cfg.DBEncryptionKey = strings.Repeat("4b", 32)
	if err := cfg.Check(true); err != nil {
		t.Errorf("a release build refused release values: %v", err)
	}
}

// The committed env files of both builds, read from this folder whatever the
// build tag: dev-only secrets, one sync secret and DB key for both nodes, no
// peer or public URL, today's ports, and examples that list every key empty.
func TestCommittedEnvFiles(t *testing.T) {
	fsys := os.DirFS(".")
	read := func(name string) map[string]string {
		t.Helper()
		env, err := envfile.Read(fsys, name)
		if err != nil {
			t.Fatal(err)
		}
		return env
	}
	local, cloud := read("local.env.defaults"), read("cloud.env.defaults")
	for name, env := range map[string]map[string]string{"local.env.defaults": local, "cloud.env.defaults": cloud} {
		if !envfile.ValidDBKey(env[envfile.KeyDBEncryptionKey]) {
			t.Errorf("%s: %s is not 64 hex characters", name, envfile.KeyDBEncryptionKey)
		}
		for _, k := range envfile.SecretKeys {
			if k != envfile.KeyDBEncryptionKey && env[k] != "" && !strings.HasPrefix(env[k], envfile.DevMarker+"-") {
				t.Errorf("%s: %s does not start with %s-", name, k, envfile.DevMarker)
			}
		}
		if len(env[envfile.KeyJWTSecret]) < envfile.MinJWTSecretLen {
			t.Errorf("%s: %s is shorter than %d characters", name, envfile.KeyJWTSecret, envfile.MinJWTSecretLen)
		}
		if env[envfile.KeyJWTLifetime] != "14h" {
			t.Errorf("%s: %s = %q, want 14h", name, envfile.KeyJWTLifetime, env[envfile.KeyJWTLifetime])
		}
		if env[envfile.KeyPeerURL] != "" || env[envfile.KeyPublicURL] != "" {
			t.Errorf("%s: PEER_URL and PUBLIC_URL must stay empty", name)
		}
		example := read(strings.TrimSuffix(name, envfile.DefaultsSuffix) + ".example")
		if !reflect.DeepEqual(sortedKeys(example), sortedKeys(env)) {
			t.Errorf("%s: the example lists %v, the defaults %v", name, sortedKeys(example), sortedKeys(env))
		}
		for k, v := range example {
			if v != "" {
				t.Errorf("%s: the example has a value for %s", name, k)
			}
		}
	}
	if local[envfile.KeyPort] != "55555" || cloud[envfile.KeyPort] != "8080" {
		t.Errorf("ports: clinic %q, cloud %q", local[envfile.KeyPort], cloud[envfile.KeyPort])
	}
	if local[envfile.KeySyncSecret] == "" || local[envfile.KeySyncSecret] != cloud[envfile.KeySyncSecret] {
		t.Error("the clinic and cloud defaults must share a non-empty SYNC_SECRET")
	}
	if local[envfile.KeyDBEncryptionKey] != cloud[envfile.KeyDBEncryptionKey] {
		t.Error("the clinic and cloud defaults must share DB_ENCRYPTION_KEY")
	}
	if local[envfile.KeyJWTSecret] == cloud[envfile.KeyJWTSecret] {
		t.Error("the clinic and cloud defaults should have different JWT secrets")
	}
	if cloud[envfile.KeyPublishSecret] == "" {
		t.Error("the cloud defaults need a PUBLISH_SECRET")
	}
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
