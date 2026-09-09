package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testDBKey is the fixed key of the throwaway test databases.
var testDBKey = strings.Repeat("0", 62) + "ff"

func TestMain(m *testing.M) {
	if err := SetKey(testDBKey); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// withoutKey clears the key for one test and restores the test key after it.
func withoutKey(t *testing.T) {
	t.Helper()
	keyMu.Lock()
	dbKey = ""
	keyMu.Unlock()
	t.Cleanup(func() {
		if err := SetKey(testDBKey); err != nil {
			t.Fatal(err)
		}
	})
}

func TestSetKey_AcceptsOnly64HexCharacters(t *testing.T) {
	t.Cleanup(func() { _ = SetKey(testDBKey) })
	bad := []string{
		"",
		strings.Repeat("a", 63),
		strings.Repeat("a", 65),
		strings.Repeat("a", 63) + "g",
		strings.Repeat("a", 62) + " a",
		strings.Repeat("é", 32),
	}
	for _, k := range bad {
		err := SetKey(k)
		if err == nil {
			t.Errorf("SetKey accepted a %d-byte key that is not 64 hex characters", len(k))
			continue
		}
		if k != "" && strings.Contains(err.Error(), k) {
			t.Errorf("the SetKey error contains the key: %v", err)
		}
	}
	for _, k := range []string{strings.Repeat("ab", 32), strings.Repeat("AB", 32), strings.Repeat("09", 32)} {
		if err := SetKey(k); err != nil {
			t.Errorf("SetKey refused a 64-hex key: %v", err)
		}
	}
}

// A rejected key leaves the previous one in place.
func TestSetKey_RejectedKeyKeepsThePreviousOne(t *testing.T) {
	if err := SetKey("not-a-key"); err == nil {
		t.Fatal("SetKey accepted an invalid key")
	}
	if got, err := currentKey(); err != nil || got != testDBKey {
		t.Fatalf("key after a rejected SetKey: changed=%v err=%v", got != testDBKey, err)
	}
}

// Without a key nothing is opened or written.
func TestOpenAndSnapshot_RequireAKey(t *testing.T) {
	dir := t.TempDir()
	withoutKey(t)
	if _, err := Open(filepath.Join(dir, "a.db")); err == nil || !strings.Contains(err.Error(), "key") {
		t.Errorf("Open without a key: %v", err)
	}
	if _, err := OpenStandalone(filepath.Join(dir, "b.db")); err == nil || !strings.Contains(err.Error(), "key") {
		t.Errorf("OpenStandalone without a key: %v", err)
	}
	if _, err := Snapshot(filepath.Join(dir, "backup")); err == nil || !strings.Contains(err.Error(), "key") {
		t.Errorf("Snapshot without a key: %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("files were created without a key: %d", len(entries))
	}
}

// Open encrypts with the key given to SetKey: the file opens with that key
// and not with another one.
func TestOpen_UsesTheKeyFromSetKey(t *testing.T) {
	key := strings.Repeat("5a", 32)
	if err := SetKey(key); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = SetKey(testDBKey) })
	path := filepath.Join(t.TempDir(), "keyed.db")
	if _, err := Open(path); err != nil {
		t.Fatal(err)
	}
	if err := Close(); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		key  string
		want bool
	}{{key, true}, {testDBKey, false}} {
		db, err := sql.Open("sqlite3", buildDSN(path, c.key))
		if err != nil {
			t.Fatal(err)
		}
		var n int
		err = db.QueryRow(`SELECT COUNT(*) FROM rooms`).Scan(&n)
		_ = db.Close()
		if (err == nil) != c.want {
			t.Errorf("read with the %s key: err=%v", map[bool]string{true: "configured", false: "test"}[c.want], err)
		}
	}
}
