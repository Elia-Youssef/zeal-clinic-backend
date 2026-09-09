package server

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"clinic-api/internal/buildmode"
)

var updateGoldens = flag.Bool("update", false, "rewrite the golden files under testdata instead of comparing with them")

// packageDir is the package source folder. It is read when the test binary
// starts, before any test moves into a temporary working directory.
var packageDir = mustGetwd()

func mustGetwd() string {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return dir
}

// buildName names the golden variant of the running build.
func buildName() string {
	if buildmode.Cloud {
		return "cloud"
	}
	return "clinic"
}

// checkGolden compares got with the golden file at rel (slash-separated,
// relative to the package folder), or rewrites the file when -update is set.
// Line endings are normalized, so a CRLF checkout compares equal.
func checkGolden(t *testing.T, rel, got string) {
	t.Helper()
	path := filepath.Join(packageDir, filepath.FromSlash(rel))
	if *updateGoldens {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("rewrote %s", rel)
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (create it with -update)", rel, err)
	}
	want := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if want != got {
		t.Errorf("%s does not match (update it with -update only for a deliberate change):\n%s", rel, lineDiff(want, got, 40))
	}
}

// lineDiff lists the lines found only in want (-) or only in got (+), at most
// limit of each. The goldens hold unique, sorted lines, so a set difference
// is enough to read a mismatch.
func lineDiff(want, got string, limit int) string {
	wantLines := strings.Split(want, "\n")
	gotLines := strings.Split(got, "\n")
	inWant := make(map[string]bool, len(wantLines))
	for _, l := range wantLines {
		inWant[l] = true
	}
	inGot := make(map[string]bool, len(gotLines))
	for _, l := range gotLines {
		inGot[l] = true
	}
	var b strings.Builder
	write := func(sign string, lines []string, other map[string]bool) {
		n := 0
		for _, l := range lines {
			if other[l] {
				continue
			}
			if n == limit {
				b.WriteString(sign + " ...\n")
				return
			}
			fmt.Fprintf(&b, "%s %s\n", sign, l)
			n++
		}
	}
	write("-", wantLines, inGot)
	write("+", gotLines, inWant)
	if b.Len() == 0 {
		return "(same lines, different order or line count)"
	}
	return b.String()
}
