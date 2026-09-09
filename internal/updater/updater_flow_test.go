package updater

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"clinic-api/internal/buildmode"
	"clinic-api/internal/database/migrations"
	"clinic-api/internal/database/store"
	"clinic-api/internal/updater/updatestate"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/vfs/adiantum"
	"github.com/pressly/goose/v3"
)

// The tests below stop every update before it is applied: applying would
// replace or exit the test binary.

type zipEntry struct{ name, body string }

func zipBytes(t *testing.T, entries ...zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, e := range entries {
		f, err := w.Create(e.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(e.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// updateServer serves files over TLS and routes the updater's download
// client to it for the test.
func updateServer(t *testing.T, files map[string][]byte) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	prev := dlClient
	dlClient = srv.Client()
	t.Cleanup(func() {
		dlClient = prev
		srv.Close()
	})
	return srv, &hits
}

func resetInstalling(t *testing.T) {
	t.Helper()
	installing.Store(false)
	t.Cleanup(func() { installing.Store(false) })
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestDownload_UsesTheUpdateClient(t *testing.T) {
	t.Chdir(t.TempDir())
	pkg := []byte("package bytes")
	srv, hits := updateServer(t, map[string][]byte{"/pkg.zip": pkg})

	path, err := download(srv.URL + "/pkg.zip")
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(downloadDir(), "ZealClinicUpdate.zip") {
		t.Errorf("download path = %s", path)
	}
	if got, _ := os.ReadFile(path); !bytes.Equal(got, pkg) {
		t.Errorf("downloaded %q, want %q", got, pkg)
	}
	if _, err := download(srv.URL + "/missing.zip"); err == nil || err.Error() != "download status 404" {
		t.Errorf("missing file: err = %v, want download status 404", err)
	}
	if hits.Load() != 2 {
		t.Errorf("server saw %d requests, want 2", hits.Load())
	}
}

func TestVerifySHA256(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pkg.zip")
	body := []byte("package bytes")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256Hex(body)
	if err := verifySHA256(path, sum); err != nil {
		t.Errorf("matching checksum: %v", err)
	}
	if err := verifySHA256(path, strings.ToUpper(sum)); err != nil {
		t.Errorf("upper-case checksum: %v", err)
	}
	other := sha256Hex([]byte("other"))
	if err := verifySHA256(path, other); err == nil || err.Error() != "sha256 mismatch: got "+sum+", want "+other {
		t.Errorf("wrong checksum: err = %v", err)
	}
	if err := verifySHA256(filepath.Join(t.TempDir(), "missing.zip"), sum); err == nil {
		t.Error("missing file verified")
	}
}

// Every entry lands directly in the emptied staging folder under its base
// name, so no entry can escape it and folders are flattened.
func TestUnzip_ExtractsFlat(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "work", "staging")
	if err := os.MkdirAll(dest, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "stale.txt"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "pkg.zip")
	if err := os.WriteFile(src, zipBytes(t,
		zipEntry{"ZealClinic.exe", "app"},
		zipEntry{"tools/", ""},
		zipEntry{"tools/bin/ZealUpdater.exe", "swapper"},
		zipEntry{"../../escape.txt", "outside"},
		zipEntry{"a/same.txt", "first"},
		zipEntry{"b/same.txt", "second"},
	), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := unzip(src, dest); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dest)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	if want := []string{"ZealClinic.exe", "ZealUpdater.exe", "escape.txt", "same.txt"}; strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("staging = %v, want %v", names, want)
	}
	for name, want := range map[string]string{"ZealClinic.exe": "app", "ZealUpdater.exe": "swapper", "escape.txt": "outside", "same.txt": "second"} {
		if got, _ := os.ReadFile(filepath.Join(dest, name)); string(got) != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	for _, outside := range []string{filepath.Join(dir, "escape.txt"), filepath.Join(dir, "work", "escape.txt")} {
		if exists(outside) {
			t.Errorf("entry escaped to %s", outside)
		}
	}

	if err := os.WriteFile(src, []byte("not a zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := unzip(src, dest); err == nil {
		t.Fatal("a broken archive was extracted")
	}
	if entries, err := os.ReadDir(dest); err != nil || len(entries) != 0 {
		t.Errorf("staging after a broken archive: %v entries, err %v; want an empty folder", len(entries), err)
	}
}

// run gives up before applying anything when the version has no checksum or
// no https URL, or when the download, the checksum or the archive fails. The
// API gate reopens and no recovery state is written.
func TestRun_StopsBeforeApplying(t *testing.T) {
	pkg := zipBytes(t, zipEntry{"ZealClinic.exe", "app"})
	cases := []struct {
		name         string
		path         string
		sha          string
		plainHTTP    bool
		wantHits     int32
		wantZip      bool
		wantStaging  bool
		checksumFrom []byte
	}{
		{name: "no checksum", path: "/pkg.zip", wantHits: 0},
		{name: "plain http", path: "/pkg.zip", plainHTTP: true, checksumFrom: pkg, wantHits: 0},
		{name: "download fails", path: "/missing.zip", checksumFrom: pkg, wantHits: 1},
		{name: "checksum mismatch", path: "/pkg.zip", checksumFrom: []byte("other"), wantHits: 1, wantZip: true},
		{name: "not an archive", path: "/broken.zip", checksumFrom: []byte("broken"), wantHits: 1, wantZip: true, wantStaging: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			resetInstalling(t)
			srv, hits := updateServer(t, map[string][]byte{"/pkg.zip": pkg, "/broken.zip": []byte("broken")})
			url := srv.URL + tc.path
			if tc.plainHTTP {
				url = "http://" + strings.TrimPrefix(url, "https://")
			}
			v := store.Version{Version: "9.9.9", Platform: Platform(), URL: url}
			if tc.checksumFrom != nil {
				v.SHA256 = sha256Hex(tc.checksumFrom)
			}

			installing.Store(true)
			run(v)
			if IsInstalling() {
				t.Error("API gate still closed after the failed update")
			}
			if hits.Load() != tc.wantHits {
				t.Errorf("download requests = %d, want %d", hits.Load(), tc.wantHits)
			}
			if got := exists(filepath.Join(downloadDir(), "ZealClinicUpdate.zip")); got != tc.wantZip {
				t.Errorf("downloaded archive kept = %v, want %v", got, tc.wantZip)
			}
			if got := exists(stagingDir()); got != tc.wantStaging {
				t.Errorf("staging folder exists = %v, want %v", got, tc.wantStaging)
			}
			if exists(statePath()) {
				t.Error("a recovery state was written")
			}
		})
	}
}

// openVersionsDB installs a migrated test database as the store pools.
func openVersionsDB(t *testing.T) *sql.DB {
	t.Helper()
	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	dsn := "file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "updater.db")) +
		"?vfs=adiantum&hexkey=" + strings.Repeat("0", 62) + "ff" +
		"&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=recursive_triggers(1)"
	w, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatal(err)
	}
	w.SetMaxOpenConns(1)
	if err := goose.Up(w, "."); err != nil {
		t.Fatal(err)
	}
	r, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatal(err)
	}
	prevDB, prevRDB := store.DB, store.RDB
	store.DB, store.RDB = w, r
	t.Cleanup(func() {
		r.Close()
		w.Close()
		store.DB, store.RDB = prevDB, prevRDB
	})
	return w
}

func TestStart_GuardsAndStatus(t *testing.T) {
	t.Chdir(t.TempDir())
	resetInstalling(t)
	db := openVersionsDB(t)

	// A fresh database lists only the running build.
	if err := Start(); !errors.Is(err, ErrNoUpdate) {
		t.Fatalf("Start on a fresh database = %v, want ErrNoUpdate", err)
	}
	st, err := GetStatus()
	if err != nil {
		t.Fatal(err)
	}
	if st.Available || st.Current != buildmode.Version || st.Latest != buildmode.Version || st.Installing {
		t.Errorf("status on a fresh database = %+v", st)
	}

	other := "linux"
	if Platform() == "linux" {
		other = "windows"
	}
	insert := func(id, version, platform, createdAt string) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO versions (id, version, platform, url, sha256, created_at) VALUES (?, ?, ?, 'http://127.0.0.1:9/pkg.zip', 'abc', ?)`,
			id, version, platform, createdAt); err != nil {
			t.Fatal(err)
		}
	}
	insert("v-900", "9.0.0", Platform(), "2099-01-01T00:00:00Z")
	insert("v-other", "10.0.0", other, "2099-06-01T00:00:00Z")
	insert("v-100", "1.0.0", Platform(), "2099-02-01T00:00:00Z")

	st, err = GetStatus()
	if err != nil {
		t.Fatal(err)
	}
	if !st.Available || st.Latest != "1.0.0" {
		t.Errorf("status = %+v, want the newest row of this platform (1.0.0) available", st)
	}

	installing.Store(true)
	if err := Start(); !errors.Is(err, ErrAlreadyInstalling) {
		t.Errorf("Start while installing = %v, want ErrAlreadyInstalling", err)
	}
	installing.Store(false)

	if err := Start(); err != nil {
		t.Fatalf("Start with a newer version = %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for IsInstalling() {
		if time.Now().After(deadline) {
			t.Fatal("the plain-http update never reopened the API gate")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if exists(statePath()) {
		t.Error("a recovery state was written")
	}
}

// finalizeOnBoot settles the recovery state left by an update.
func TestFinalizeOnBoot_StateTransitions(t *testing.T) {
	cases := []struct {
		name           string
		state          *updatestate.State
		raw            string
		wantInstalling bool
		wantCleaned    bool
		wantStateKept  bool
	}{
		{name: "success", state: &updatestate.State{Phase: updatestate.Success, Target: buildmode.Version}, wantCleaned: true},
		{name: "failed", state: &updatestate.State{Phase: updatestate.Failed, Target: "9.9.9", Error: "health check failed"}, wantCleaned: true},
		{name: "applying, same build", state: &updatestate.State{Phase: updatestate.Applying, Target: buildmode.Version}, wantCleaned: true},
		{name: "applying, other build", state: &updatestate.State{Phase: updatestate.Applying, Target: "9.9.9"}, wantCleaned: true},
		{name: "trial", state: &updatestate.State{Phase: updatestate.Trial, Target: "9.9.9"}, wantInstalling: true, wantStateKept: true},
		{name: "no state"},
		{name: "unreadable state", raw: "{not json", wantStateKept: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			resetInstalling(t)
			artifacts := []string{
				filepath.Join(downloadDir(), "ZealClinic.exe.bak"),
				filepath.Join(downloadDir(), "clinic.db.snap"),
				filepath.Join(downloadDir(), "ZealClinicUpdate.zip"),
				runnerPath(),
				filepath.Join(stagingDir(), "ZealClinic.exe"),
			}
			if err := os.MkdirAll(stagingDir(), 0o700); err != nil {
				t.Fatal(err)
			}
			for _, p := range artifacts {
				if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			sp := statePath()
			switch {
			case tc.state != nil:
				st := *tc.state
				st.BackupExe, st.DBSnapshot, st.ZipPath, st.StagingDir = artifacts[0], artifacts[1], artifacts[2], stagingDir()
				if err := updatestate.Write(sp, st); err != nil {
					t.Fatal(err)
				}
			case tc.raw != "":
				if err := os.WriteFile(sp, []byte(tc.raw), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			FinalizeOnBoot(false)

			if IsInstalling() != tc.wantInstalling {
				t.Errorf("installing = %v, want %v", IsInstalling(), tc.wantInstalling)
			}
			if got := exists(sp); got != tc.wantStateKept {
				t.Errorf("state file kept = %v, want %v", got, tc.wantStateKept)
			}
			for _, p := range artifacts {
				if exists(p) == tc.wantCleaned {
					t.Errorf("%s exists = %v, want cleaned = %v", filepath.Base(p), exists(p), tc.wantCleaned)
				}
			}
			if exists(stagingDir()) == tc.wantCleaned {
				t.Errorf("staging folder exists = %v, want cleaned = %v", exists(stagingDir()), tc.wantCleaned)
			}
		})
	}
}
