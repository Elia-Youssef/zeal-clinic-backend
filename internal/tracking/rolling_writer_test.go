package tracking

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRollingWriterRotatesAndRetainsBoundedBackups(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	w, err := newRollingWriter(path, 5, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"aaaa", "bbbb", "cccc", "dddd"} {
		if _, err := w.Write([]byte(s)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{path, path + ".1", path + ".2"} {
		if _, err := os.Stat(name); err != nil {
			t.Fatalf("expected %s: %v", name, err)
		}
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Fatalf("unexpected third backup: %v", err)
	}
}
