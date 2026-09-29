//go:build cloud

package cloudrestore

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"clinic-api/internal/buildmode"
	"clinic-api/internal/config"
	"clinic-api/internal/database"
	"clinic-api/internal/database/store"
	"clinic-api/internal/realtime"
	syncpkg "clinic-api/internal/sync"

	"github.com/labstack/echo/v4"
)

const cloudRestoreSecret = "test-sync-secret"

type recordingRuntime struct {
	mu      sync.Mutex
	paused  int
	resumed []*sql.DB
}

func (r *recordingRuntime) Pause() {
	r.mu.Lock()
	r.paused++
	r.mu.Unlock()
}

func (r *recordingRuntime) Resume(db *sql.DB) {
	r.mu.Lock()
	r.resumed = append(r.resumed, db)
	r.mu.Unlock()
}

type cloudRestoreEnv struct {
	api         *API
	e           *echo.Echo
	runtime     *recordingRuntime
	invalidated int
}

// newCloudRestoreEnv moves to a temp working directory and mounts the restore
// endpoint behind the maintenance gate, as the cloud server does.
func newCloudRestoreEnv(t *testing.T) *cloudRestoreEnv {
	t.Helper()
	t.Chdir(t.TempDir())
	env := &cloudRestoreEnv{runtime: &recordingRuntime{}}
	env.api = New(Config{Secret: cloudRestoreSecret, Invalidate: func() { env.invalidated++ }})
	env.e = echo.New()
	env.e.Use(env.api.Middleware())
	env.e.POST("/api/cloud-restore", env.api.HandleCloud)
	env.e.GET("/api/ping", func(c echo.Context) error { return c.NoContent(http.StatusNoContent) })
	SetRuntime(env.runtime)
	t.Cleanup(func() { SetRuntime(nil) })
	return env
}

// localSnapshot builds a clinic database in ./local, lets prepare change it,
// and returns the bytes of its snapshot, as the clinic uploads them.
func localSnapshot(t *testing.T, prepare func(db *sql.DB)) []byte {
	t.Helper()
	db, err := database.Open(filepath.Join("local", "clinic.db"))
	if err != nil {
		t.Fatal(err)
	}
	if prepare != nil {
		prepare(db)
	}
	path, err := database.Snapshot(filepath.Join("local", "export"))
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// openCloud opens the cloud database at the build's default path.
func openCloud(t *testing.T, prepare func(db *sql.DB)) *sql.DB {
	t.Helper()
	db, err := database.Open("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if prepare != nil {
		prepare(db)
	}
	return db
}

type restoreReply struct {
	status  int
	Success bool        `json:"success"`
	Data    ApplyResult `json:"data"`
	Error   string      `json:"error"`
}

// post uploads body with valid headers; overrides replace or, when empty,
// remove single headers.
func (env *cloudRestoreEnv) post(t *testing.T, body []byte, overrides map[string]string) restoreReply {
	t.Helper()
	sum := sha256.Sum256(body)
	headers := map[string]string{
		"X-Sync-Secret":    cloudRestoreSecret,
		"X-Sync-Version":   buildmode.Version,
		"X-Restore-SHA256": hex.EncodeToString(sum[:]),
		"X-Restore-ID":     "restore-test-1",
	}
	for k, v := range overrides {
		headers[k] = v
	}
	req := httptest.NewRequest(http.MethodPost, "/api/cloud-restore", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/octet-stream")
	for k, v := range headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}
	rec := httptest.NewRecorder()
	env.e.ServeHTTP(rec, req)
	reply := restoreReply{status: rec.Code}
	if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil {
		t.Fatalf("decode reply %q: %v", rec.Body.String(), err)
	}
	return reply
}

func (env *cloudRestoreEnv) gateIsOpen(t *testing.T) bool {
	t.Helper()
	rec := httptest.NewRecorder()
	env.e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/ping", nil))
	return rec.Code == http.StatusNoContent
}

func dirEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func progressStages(t *testing.T, client *realtime.Client) []string {
	t.Helper()
	var stages []string
	for {
		select {
		case ev := <-client.Events():
			if p, ok := ev.Data.(Progress); ok && ev.Type == progressEvent {
				stages = append(stages, p.Status+":"+p.Stage)
			}
		default:
			return stages
		}
	}
}

func TestHandleCloud_ChecksHeadersBeforeReadingTheDump(t *testing.T) {
	env := newCloudRestoreEnv(t)
	body := []byte("not read")
	cases := []struct {
		name      string
		overrides map[string]string
		length    int64
		status    int
		err       string
	}{
		{"no secret", map[string]string{"X-Sync-Secret": ""}, 0, http.StatusUnauthorized, "Not authorized"},
		{"wrong secret", map[string]string{"X-Sync-Secret": "wrong", "X-Sync-Version": "0.0.0"}, 0, http.StatusUnauthorized, "Not authorized"},
		{"no version", map[string]string{"X-Sync-Version": ""}, 0, http.StatusConflict, "Version mismatch"},
		{"other version", map[string]string{"X-Sync-Version": "0.0.0"}, 0, http.StatusConflict, "Version mismatch"},
		{"too large", nil, maxDumpBytes + 1, http.StatusRequestEntityTooLarge, "Database dump is too large"},
		{"no checksum", map[string]string{"X-Restore-SHA256": ""}, 0, http.StatusBadRequest, "Missing or invalid dump checksum"},
		{"short checksum", map[string]string{"X-Restore-SHA256": "abc123"}, 0, http.StatusBadRequest, "Missing or invalid dump checksum"},
		{"no restore id", map[string]string{"X-Restore-ID": ""}, 0, http.StatusBadRequest, "Missing or invalid restore id"},
		{"long restore id", map[string]string{"X-Restore-ID": strings.Repeat("r", 129)}, 0, http.StatusBadRequest, "Missing or invalid restore id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sum := sha256.Sum256(body)
			headers := map[string]string{
				"X-Sync-Secret":    cloudRestoreSecret,
				"X-Sync-Version":   buildmode.Version,
				"X-Restore-SHA256": hex.EncodeToString(sum[:]),
				"X-Restore-ID":     "restore-test-1",
			}
			for k, v := range tc.overrides {
				headers[k] = v
			}
			req := httptest.NewRequest(http.MethodPost, "/api/cloud-restore", bytes.NewReader(body))
			for k, v := range headers {
				if v != "" {
					req.Header.Set(k, v)
				}
			}
			if tc.length > 0 {
				req.ContentLength = tc.length
			}
			rec := httptest.NewRecorder()
			env.e.ServeHTTP(rec, req)
			want := `{"error":"` + tc.err + `"}`
			if rec.Code != tc.status || strings.TrimSpace(rec.Body.String()) != want {
				t.Errorf("status %d %s, want %d %s", rec.Code, strings.TrimSpace(rec.Body.String()), tc.status, want)
			}
		})
	}
	if env.runtime.paused != 0 || !env.gateIsOpen(t) {
		t.Errorf("rejected requests paused services %d times or left the gate closed", env.runtime.paused)
	}
	if names := dirEntries(t, filepath.Join(config.DataDir(), "cloud-restore", "incoming")); len(names) != 0 {
		t.Errorf("staging files after rejected requests: %v", names)
	}

	noSecret := New(Config{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/cloud-restore", nil)
	if err := noSecret.HandleCloud(echo.New().NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("without a configured secret: status %d, want 401", rec.Code)
	}
}

// A second restore is refused while one holds the maintenance gate: by the
// handler itself (409) and by the gate in front of it (503).
func TestHandleCloud_RefusesARestoreInProgress(t *testing.T) {
	env := newCloudRestoreEnv(t)
	if err := env.api.gate.begin(); err != nil {
		t.Fatal(err)
	}
	defer env.api.gate.end()

	body := []byte("dump")
	sum := sha256.Sum256(body)
	req := httptest.NewRequest(http.MethodPost, "/api/cloud-restore", bytes.NewReader(body))
	req.Header.Set("X-Sync-Secret", cloudRestoreSecret)
	req.Header.Set("X-Sync-Version", buildmode.Version)
	req.Header.Set("X-Restore-SHA256", hex.EncodeToString(sum[:]))
	req.Header.Set("X-Restore-ID", "restore-test-2")
	rec := httptest.NewRecorder()
	if err := env.api.HandleCloud(env.e.NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusConflict || strings.TrimSpace(rec.Body.String()) != `{"error":"cloud restore already in progress"}` {
		t.Errorf("handler during a restore: %d %s", rec.Code, rec.Body.String())
	}

	reply := env.post(t, body, nil)
	if reply.status != http.StatusServiceUnavailable || reply.Error != "Cloud restore in progress, please wait" {
		t.Errorf("request through the gate during a restore: %d %q", reply.status, reply.Error)
	}
	if env.runtime.paused != 0 {
		t.Errorf("services paused %d times", env.runtime.paused)
	}
}

func TestHandleCloud_ReplacesSyncedTablesFromTheSnapshot(t *testing.T) {
	env := newCloudRestoreEnv(t)
	var localBaseline int64
	dump := localSnapshot(t, func(db *sql.DB) {
		mustExec(t, db, `INSERT INTO rooms (id, name, type) VALUES ('local-room', 'Local Room', 'General')`)
		var err error
		if localBaseline, err = outboxHighWater(db); err != nil {
			t.Fatal(err)
		}
	})
	var cloudBaseline int64
	oldPool := openCloud(t, func(db *sql.DB) {
		mustExec(t, db, `INSERT INTO rooms (id, name, type) VALUES ('cloud-room', 'Cloud Room', 'General')`)
		mustExec(t, db, `INSERT INTO users (id, username, display_name, role) VALUES ('cloud-user', 'cloud-user', 'Cloud User', 'staff')`)
		mustExec(t, db, `INSERT INTO tokens (id, token, user_id, expires_at) VALUES ('token-kept', 'token-kept', ?, '2099-01-01T00:00:00Z')`, seededSuperAdminID)
		mustExec(t, db, `INSERT INTO tokens (id, token, user_id, expires_at) VALUES ('token-dropped', 'token-dropped', 'cloud-user', '2099-01-01T00:00:00Z')`)
		mustExec(t, db, `INSERT INTO notifications (id, user_id, title) VALUES ('note-kept', ?, 'Kept')`, seededSuperAdminID)
		mustExec(t, db, `INSERT INTO sync_conflicts (id, table_name, row_id) VALUES ('conflict-1', 'rooms', 'cloud-room')`)
		var err error
		if cloudBaseline, err = outboxHighWater(db); err != nil {
			t.Fatal(err)
		}
	})
	client := realtime.Register("cloud-restore-cloud-test")
	defer client.Close()

	sum := sha256.Sum256(dump)
	reply := env.post(t, dump, map[string]string{"X-Restore-SHA256": strings.ToUpper(hex.EncodeToString(sum[:]))})
	if reply.status != http.StatusOK || !reply.Success || reply.Error != "" {
		t.Fatalf("restore = %d %+v", reply.status, reply)
	}

	db := store.DB
	if db == nil || db == oldPool {
		t.Fatal("the cloud database was not reopened")
	}
	var rows int64
	for _, table := range syncpkg.SyncedTables {
		var n int64
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + quoteIdent(table.Name)).Scan(&n); err != nil {
			t.Fatal(err)
		}
		rows += n
	}
	backups := dirEntries(t, config.BackupDir())
	if len(backups) != 1 {
		t.Fatalf("backups = %v, want one", backups)
	}
	want := ApplyResult{Tables: len(syncpkg.SyncedTables), Rows: rows, LocalBaseline: localBaseline, CloudBaseline: cloudBaseline, Backup: backups[0], RestoreID: "restore-test-1"}
	if reply.Data != want {
		t.Errorf("result = %+v, want %+v", reply.Data, want)
	}

	assertCount(t, db, 1, `SELECT COUNT(*) FROM rooms WHERE id = 'local-room'`)
	assertCount(t, db, 0, `SELECT COUNT(*) FROM rooms WHERE id = 'cloud-room'`)
	assertCount(t, db, 1, `SELECT COUNT(*) FROM tokens WHERE id = 'token-kept'`)
	assertCount(t, db, 0, `SELECT COUNT(*) FROM tokens WHERE id = 'token-dropped'`)
	assertCount(t, db, 1, `SELECT COUNT(*) FROM notifications WHERE id = 'note-kept'`)
	assertCount(t, db, 0, `SELECT COUNT(*) FROM sync_log`)
	assertCount(t, db, 0, `SELECT COUNT(*) FROM sync_conflicts`)
	var pushed, pulled int64
	if err := db.QueryRow(`SELECT last_pushed_seq, last_pulled_seq FROM sync_state WHERE peer = 'peer'`).Scan(&pushed, &pulled); err != nil {
		t.Fatal(err)
	}
	if pushed != cloudBaseline || pulled != localBaseline {
		t.Errorf("sync state = pushed %d pulled %d, want %d and %d", pushed, pulled, cloudBaseline, localBaseline)
	}

	backup, err := database.OpenStandalone(filepath.Join(config.BackupDir(), backups[0]))
	if err != nil {
		t.Fatal(err)
	}
	defer backup.Close()
	assertCount(t, backup, 1, `SELECT COUNT(*) FROM rooms WHERE id = 'cloud-room'`)

	if names := dirEntries(t, filepath.Join(config.DataDir(), "cloud-restore", "incoming")); len(names) != 0 {
		t.Errorf("staging files left: %v", names)
	}
	if env.runtime.paused != 1 || len(env.runtime.resumed) != 1 || env.runtime.resumed[0] != db {
		t.Errorf("services paused %d times, resumed with %v, want once with the reopened pool", env.runtime.paused, env.runtime.resumed)
	}
	if env.invalidated != 1 {
		t.Errorf("cache invalidated %d times, want 1", env.invalidated)
	}
	if !env.gateIsOpen(t) {
		t.Error("maintenance gate still closed")
	}
	wantStages := []string{"running:receiving", "running:validating", "running:backing_up", "running:applying", "running:reopening", "success:completed"}
	if got := progressStages(t, client); strings.Join(got, ",") != strings.Join(wantStages, ",") {
		t.Errorf("progress = %v, want %v", got, wantStages)
	}
}

// Failures before the backup leave the cloud database untouched and reopen
// the gate; the reply is a 500 with the reason.
func TestHandleCloud_RejectsBadDumps(t *testing.T) {
	cases := []struct {
		name    string
		prepare func(t *testing.T, db *sql.DB)
		corrupt func(dump []byte) ([]byte, map[string]string)
		err     string
	}{
		{
			name: "checksum mismatch",
			corrupt: func(dump []byte) ([]byte, map[string]string) {
				other := sha256.Sum256([]byte("another dump"))
				return dump, map[string]string{"X-Restore-SHA256": hex.EncodeToString(other[:])}
			},
			err: "database dump checksum mismatch",
		},
		{
			name: "older schema version",
			prepare: func(t *testing.T, db *sql.DB) {
				mustExec(t, db, `DELETE FROM goose_db_version WHERE version_id = (SELECT MAX(version_id) FROM goose_db_version)`)
			},
			err: "schema version mismatch: local=14 cloud=15",
		},
		{
			name: "changed table",
			prepare: func(t *testing.T, db *sql.DB) {
				mustExec(t, db, `ALTER TABLE rooms ADD COLUMN floor TEXT NOT NULL DEFAULT ''`)
			},
			err: "table schema mismatch: rooms",
		},
		{
			name: "not a database",
			corrupt: func([]byte) ([]byte, map[string]string) {
				return bytes.Repeat([]byte("not a database "), 512), nil
			},
			err: "uploaded dump",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newCloudRestoreEnv(t)
			dump := localSnapshot(t, func(db *sql.DB) {
				mustExec(t, db, `INSERT INTO rooms (id, name, type) VALUES ('local-room', 'Local Room', 'General')`)
				if tc.prepare != nil {
					tc.prepare(t, db)
				}
			})
			var overrides map[string]string
			if tc.corrupt != nil {
				dump, overrides = tc.corrupt(dump)
			}
			openCloud(t, func(db *sql.DB) {
				mustExec(t, db, `INSERT INTO rooms (id, name, type) VALUES ('cloud-room', 'Cloud Room', 'General')`)
			})

			reply := env.post(t, dump, overrides)
			if reply.status != http.StatusInternalServerError || !strings.Contains(reply.Error, tc.err) {
				t.Fatalf("restore = %d %q, want 500 containing %q", reply.status, reply.Error, tc.err)
			}
			assertCount(t, store.DB, 1, `SELECT COUNT(*) FROM rooms WHERE id = 'cloud-room'`)
			assertCount(t, store.DB, 0, `SELECT COUNT(*) FROM rooms WHERE id = 'local-room'`)
			if b := dirEntries(t, config.BackupDir()); len(b) != 0 {
				t.Errorf("backups taken: %v", b)
			}
			if names := dirEntries(t, filepath.Join(config.DataDir(), "cloud-restore", "incoming")); len(names) != 0 {
				t.Errorf("staging files left: %v", names)
			}
			if env.runtime.paused != 1 || len(env.runtime.resumed) != 1 || env.invalidated != 0 {
				t.Errorf("paused %d, resumed %d, invalidated %d; want 1, 1, 0", env.runtime.paused, len(env.runtime.resumed), env.invalidated)
			}
			if !env.gateIsOpen(t) {
				t.Error("maintenance gate still closed")
			}
		})
	}
}

// A failure while replacing the tables restores the backup taken just before
// and reopens the database.
func TestHandleCloud_RollsBackToThePreRestoreBackup(t *testing.T) {
	env := newCloudRestoreEnv(t)
	dump := localSnapshot(t, func(db *sql.DB) {
		mustExec(t, db, `INSERT INTO rooms (id, name, type) VALUES ('local-room', 'Local Room', 'General')`)
		mustExec(t, db, `INSERT INTO rooms (id, name, type) VALUES ('poison-room', 'Poison Room', 'General')`)
	})
	openCloud(t, func(db *sql.DB) {
		mustExec(t, db, `INSERT INTO rooms (id, name, type) VALUES ('cloud-room', 'Cloud Room', 'General')`)
		mustExec(t, db, `CREATE TRIGGER restore_poison BEFORE INSERT ON rooms WHEN NEW.id = 'poison-room'
			BEGIN SELECT RAISE(ABORT, 'poisoned row'); END`)
	})
	client := realtime.Register("cloud-restore-rollback-test")
	defer client.Close()

	reply := env.post(t, dump, nil)
	if reply.status != http.StatusInternalServerError ||
		!strings.HasPrefix(reply.Error, "insert rooms: ") ||
		!strings.Contains(reply.Error, "poisoned row") ||
		!strings.HasSuffix(reply.Error, "(cloud database rolled back)") {
		t.Fatalf("restore = %d %q", reply.status, reply.Error)
	}
	db := store.DB
	if db == nil {
		t.Fatal("cloud database left closed")
	}
	assertCount(t, db, 1, `SELECT COUNT(*) FROM rooms WHERE id = 'cloud-room'`)
	assertCount(t, db, 0, `SELECT COUNT(*) FROM rooms WHERE id IN ('local-room', 'poison-room')`)
	if b := dirEntries(t, config.BackupDir()); len(b) != 1 {
		t.Errorf("backups = %v, want the one taken before applying", b)
	}
	if env.runtime.paused != 1 || len(env.runtime.resumed) != 1 || env.runtime.resumed[0] != db {
		t.Errorf("paused %d, resumed %v; want once with the reopened pool", env.runtime.paused, env.runtime.resumed)
	}
	if !env.gateIsOpen(t) {
		t.Error("maintenance gate still closed")
	}
	wantStages := []string{"running:receiving", "running:validating", "running:backing_up", "running:applying", "running:rolling_back", "failed:failed"}
	if got := progressStages(t, client); strings.Join(got, ",") != strings.Join(wantStages, ",") {
		t.Errorf("progress = %v, want %v", got, wantStages)
	}
}
