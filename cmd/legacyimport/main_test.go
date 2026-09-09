package main

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"clinic-api/internal/config"
	"clinic-api/internal/legacyimport"
)

// The importer takes the DB key from the config: the database it writes opens
// with that key and not with another one, and an invalid key stops it before
// anything is opened, with an error that names the key but not its value.
func TestUseConfigKey(t *testing.T) {
	// The fixtures of the importer's own tests.
	fixtures, err := filepath.Abs(filepath.Join("..", "..", "internal", "legacyimport", "testdata", "legacy"))
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	cfg := config.Load()

	cfg.DBEncryptionKey = strings.Repeat("x", 20)
	err = useConfigKey(cfg)
	if err == nil {
		t.Fatal("useConfigKey accepted an invalid DB key")
	}
	if !strings.Contains(err.Error(), "DB_ENCRYPTION_KEY") || strings.Contains(err.Error(), cfg.DBEncryptionKey) {
		t.Errorf("error = %v", err)
	}

	key := strings.Repeat("7e", 32)
	cfg.DBEncryptionKey = key
	if err := useConfigKey(cfg); err != nil {
		t.Fatal(err)
	}
	path, err := filepath.Abs("clinic.db")
	if err != nil {
		t.Fatal(err)
	}
	if err := legacyimport.Run(legacyimport.Options{InputDir: fixtures, DBPath: path, ReportPath: "report.txt"}); err != nil {
		t.Fatalf("import with the configured key: %v", err)
	}

	for _, c := range []struct {
		key  string
		want bool
	}{{key, true}, {strings.Repeat("0", 62) + "ff", false}} {
		db, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(path)+"?vfs=adiantum&hexkey="+c.key)
		if err != nil {
			t.Fatal(err)
		}
		var n int
		err = db.QueryRow(`SELECT COUNT(*) FROM patients`).Scan(&n)
		_ = db.Close()
		if c.want && (err != nil || n == 0) {
			t.Errorf("read with the configured key: %d patients, err %v", n, err)
		}
		if !c.want && err == nil {
			t.Error("the imported database opened with another key")
		}
	}
}
