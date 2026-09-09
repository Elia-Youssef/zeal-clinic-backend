package envfile

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
)

// The override replaces the defaults key by key: a non-empty value wins, an
// empty one keeps the default, a key only in the override is added.
func TestLoad_OverrideAppliesKeyByKey(t *testing.T) {
	fsys := fstest.MapFS{
		"node.env.defaults": {Data: []byte("# defaults\nPORT=1000\nJWT_SECRET=default1\nPEER_URL=\nSYNC_SECRET=default2\n")},
		"node.env":          {Data: []byte("JWT_SECRET=override1\r\nSYNC_SECRET=\r\nPEER_URL=http://127.0.0.1:9\r\nEXTRA=1\r\n")},
	}
	env, set, err := Load(fsys, "node.env")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"PORT": "1000", "JWT_SECRET": "override1", "PEER_URL": "http://127.0.0.1:9",
		"SYNC_SECRET": "default2", "EXTRA": "1",
	}
	if !reflect.DeepEqual(env, want) {
		t.Errorf("merged env = %v, want %v", env, want)
	}
	if strings.Join(set, ",") != "EXTRA,JWT_SECRET,PEER_URL" {
		t.Errorf("keys set by the override = %v", set)
	}

	delete(fsys, "node.env")
	env, set, err = Load(fsys, "node.env")
	if err != nil {
		t.Fatal(err)
	}
	if env["JWT_SECRET"] != "default1" || len(set) != 0 {
		t.Errorf("without an override: env %v, set %v", env, set)
	}
}

func TestLoad_Errors(t *testing.T) {
	if _, _, err := Load(fstest.MapFS{"node.env": {Data: []byte("PORT=1\n")}}, "node.env"); err == nil {
		t.Error("Load without the defaults file succeeded")
	}
	// The parser's own message would quote the rest of the file.
	leak := strings.Repeat("q", 12)
	fsys := fstest.MapFS{
		"node.env.defaults": {Data: []byte("PORT=1\n")},
		"node.env":          {Data: []byte("JWT SECRET " + leak + "!\n")},
	}
	_, _, err := Load(fsys, "node.env")
	if err == nil {
		t.Fatal("Load accepted an override with invalid syntax")
	}
	if strings.Contains(err.Error(), leak) {
		t.Errorf("the syntax error quotes the file: %v", err)
	}
}

// releaseValues are non-dev values that pass the release rules.
func releaseValues() map[string]string {
	return map[string]string{
		KeyJWTSecret:       strings.Repeat("j", MinJWTSecretLen),
		KeySyncSecret:      strings.Repeat("s", 40),
		KeyPublishSecret:   strings.Repeat("p", 40),
		KeyDBEncryptionKey: strings.Repeat("4b", 32),
	}
}

func TestProblems_ReleaseRules(t *testing.T) {
	defaults := map[string]string{
		KeyJWTSecret: DevMarker + "-jwt-default-value-for-the-tests", KeySyncSecret: DevMarker + "-sync",
		KeyDBEncryptionKey: strings.Repeat("0d", 32),
	}
	cases := []struct {
		name    string
		change  map[string]string
		release bool
		want    []string
	}{
		{"release values", nil, true, nil},
		{"release values in a dev build", nil, false, nil},
		{"the default JWT secret", map[string]string{KeyJWTSecret: defaults[KeyJWTSecret]}, true, []string{KeyJWTSecret + " holds a dev value"}},
		{"the default DB key in upper case", map[string]string{KeyDBEncryptionKey: strings.ToUpper(defaults[KeyDBEncryptionKey])}, true, []string{KeyDBEncryptionKey + " holds a dev value"}},
		{"any value marked dev-only", map[string]string{KeyPublishSecret: "xx-DEV-ONLY-xx"}, true, []string{KeyPublishSecret + " holds a dev value"}},
		{"dev values in a dev build", map[string]string{KeySyncSecret: defaults[KeySyncSecret]}, false, nil},
		{"a short JWT secret", map[string]string{KeyJWTSecret: strings.Repeat("j", MinJWTSecretLen-1)}, true, []string{KeyJWTSecret + " must be at least 32 characters"}},
		{"a short JWT secret in a dev build", map[string]string{KeyJWTSecret: "short"}, false, nil},
		{"a DB key of 63 characters", map[string]string{KeyDBEncryptionKey: strings.Repeat("a", 63)}, false, []string{KeyDBEncryptionKey + " must be 64 hex characters"}},
		{"a DB key that isn't hex", map[string]string{KeyDBEncryptionKey: strings.Repeat("g", 64)}, true, []string{KeyDBEncryptionKey + " must be 64 hex characters"}},
		{"no DB key", map[string]string{KeyDBEncryptionKey: ""}, false, []string{KeyDBEncryptionKey + " must be 64 hex characters"}},
		{"an empty secret without a default", map[string]string{KeyPublishSecret: ""}, true, nil},
	}
	for _, c := range cases {
		env := releaseValues()
		for k, v := range c.change {
			env[k] = v
		}
		got := Problems(env, defaults, c.release)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: problems = %q, want %q", c.name, got, c.want)
		}
		assertNoValues(t, got, env)
	}
}

func TestValidDBKey(t *testing.T) {
	for _, k := range []string{strings.Repeat("ab", 32), strings.Repeat("AB", 32), strings.Repeat("09", 32)} {
		if !ValidDBKey(k) {
			t.Errorf("a 64-hex key was refused")
		}
	}
	for _, k := range []string{"", strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("a", 63) + "g", strings.Repeat("é", 32)} {
		if ValidDBKey(k) {
			t.Errorf("a %d-byte key that is not 64 hex characters was accepted", len(k))
		}
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

// assertNoValues fails when a problem quotes a value of the given env maps.
func assertNoValues(t *testing.T, problems []string, envs ...map[string]string) {
	t.Helper()
	for _, p := range problems {
		for _, env := range envs {
			for k, v := range env {
				if len(v) >= 8 && strings.Contains(p, v) {
					t.Errorf("%q quotes the value of %s", p, k)
				}
			}
		}
	}
}

// The pre-build check on a copy of the committed defaults (in the config
// folder above) and generated overrides.
func TestCheckRelease(t *testing.T) {
	src := os.DirFS("..")
	newDir := func(t *testing.T) string {
		t.Helper()
		dir := t.TempDir()
		for _, f := range []string{"local.env.defaults", "cloud.env.defaults"} {
			b, err := fs.ReadFile(src, f)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, f), b, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}
	write := func(t *testing.T, dir, name string, env map[string]string) {
		t.Helper()
		var b strings.Builder
		for _, k := range sortedKeys(env) {
			b.WriteString(k + "=" + env[k] + "\n")
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(b.String()), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	good := func() (map[string]string, map[string]string) {
		shared := map[string]string{KeySyncSecret: strings.Repeat("s", 48), KeyDBEncryptionKey: strings.Repeat("c3", 32)}
		clinic := map[string]string{KeyJWTSecret: strings.Repeat("j", 64), KeyPeerURL: "https://cloud.example.invalid"}
		cloud := map[string]string{KeyJWTSecret: strings.Repeat("k", 64), KeyPublishSecret: strings.Repeat("p", 48)}
		for k, v := range shared {
			clinic[k], cloud[k] = v, v
		}
		return clinic, cloud
	}
	both := []string{"clinic", "cloud"}

	t.Run("release values pass", func(t *testing.T) {
		dir := newDir(t)
		clinic, cloud := good()
		write(t, dir, "local.env", clinic)
		write(t, dir, "cloud.env", cloud)
		for _, nodes := range [][]string{both, {"clinic"}, {"cloud"}} {
			if p, err := CheckRelease(dir, nodes); err != nil || len(p) != 0 {
				t.Errorf("%v: problems %q, err %v", nodes, p, err)
			}
		}
	})

	t.Run("missing overrides", func(t *testing.T) {
		dir := newDir(t)
		p, err := CheckRelease(dir, both)
		if err != nil {
			t.Fatal(err)
		}
		if len(p) != 2 || !strings.HasPrefix(p[0], "local.env: missing") || !strings.HasPrefix(p[1], "cloud.env: missing") {
			t.Errorf("problems = %q", p)
		}
		// Only the node being built needs its override.
		_, cloud := good()
		write(t, dir, "cloud.env", cloud)
		if p, err := CheckRelease(dir, []string{"cloud"}); err != nil || len(p) != 0 {
			t.Errorf("cloud only: problems %q, err %v", p, err)
		}
	})

	t.Run("dev values, a short JWT secret and a bad DB key", func(t *testing.T) {
		dir := newDir(t)
		clinic, cloud := good()
		delete(clinic, KeyJWTSecret) // the dev default would be embedded
		clinic[KeySyncSecret] = DevMarker + "-sync-secret-not-for-release"
		cloud[KeySyncSecret] = clinic[KeySyncSecret]
		cloud[KeyJWTSecret] = strings.Repeat("k", 31)
		cloud[KeyDBEncryptionKey] = strings.Repeat("z", 64)
		clinic[KeyDBEncryptionKey] = cloud[KeyDBEncryptionKey]
		write(t, dir, "local.env", clinic)
		write(t, dir, "cloud.env", cloud)
		p, err := CheckRelease(dir, both)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{
			"local.env: DB_ENCRYPTION_KEY must be 64 hex characters",
			"local.env: JWT_SECRET holds a dev value",
			"local.env: SYNC_SECRET holds a dev value",
			"cloud.env: DB_ENCRYPTION_KEY must be 64 hex characters",
			"cloud.env: SYNC_SECRET holds a dev value",
			"cloud.env: JWT_SECRET must be at least 32 characters",
		}
		if !reflect.DeepEqual(p, want) {
			t.Errorf("problems =\n%q\nwant\n%q", p, want)
		}
		assertNoValues(t, p, clinic, cloud)
	})

	t.Run("the nodes must share the sync secret and the DB key", func(t *testing.T) {
		dir := newDir(t)
		clinic, cloud := good()
		cloud[KeySyncSecret] = strings.Repeat("t", 48)
		cloud[KeyDBEncryptionKey] = strings.Repeat("d4", 32)
		write(t, dir, "local.env", clinic)
		write(t, dir, "cloud.env", cloud)
		for _, nodes := range [][]string{both, {"clinic"}} {
			p, err := CheckRelease(dir, nodes)
			if err != nil {
				t.Fatal(err)
			}
			want := []string{"SYNC_SECRET differs between local.env and cloud.env", "DB_ENCRYPTION_KEY differs between local.env and cloud.env"}
			if !reflect.DeepEqual(p, want) {
				t.Errorf("%v: problems = %q", nodes, p)
			}
			assertNoValues(t, p, clinic, cloud)
		}
		// The same key in another case is the same key.
		cloud[KeySyncSecret] = clinic[KeySyncSecret]
		cloud[KeyDBEncryptionKey] = strings.ToUpper(clinic[KeyDBEncryptionKey])
		write(t, dir, "cloud.env", cloud)
		if p, err := CheckRelease(dir, both); err != nil || len(p) != 0 {
			t.Errorf("same key in upper case: problems %q, err %v", p, err)
		}
	})

	t.Run("bad arguments", func(t *testing.T) {
		dir := newDir(t)
		if _, err := CheckRelease(dir, nil); err == nil {
			t.Error("no node: no error")
		}
		if _, err := CheckRelease(dir, []string{"server"}); err == nil {
			t.Error("unknown node: no error")
		}
		// A folder without any env file only reports the missing override.
		if p, err := CheckRelease(t.TempDir(), []string{"clinic"}); err != nil || len(p) != 1 {
			t.Errorf("empty folder: problems %q, err %v", p, err)
		}
	})
}
