package server

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	mw "clinic-api/internal/api/middleware"
	"clinic-api/internal/config"
	"clinic-api/internal/database/migrations"
	"clinic-api/internal/database/store"

	"github.com/labstack/echo/v4"
	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/vfs/adiantum"
	"github.com/pressly/goose/v3"
)

const testHexKey = "00000000000000000000000000000000000000000000000000000000000000ff"

func testDSN(dbPath string) string {
	return "file:" + filepath.ToSlash(dbPath) +
		"?vfs=adiantum" +
		"&hexkey=" + testHexKey +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(1)"
}

// DB harness

var (
	dbCounter        atomic.Uint64
	gooseInitOnce    sync.Once
	configInitOnce   sync.Once
)

// setupTestEnv opens a fresh on-disk SQLite, runs migrations, swaps the
// package-level store.DB / store.RDB handles, and ensures config.Load has
// been called (so JWT_SECRET etc. are populated). Caller-side cleanup is
// registered via t.Cleanup.
func setupTestEnv(t *testing.T) {
	t.Helper()

	gooseInitOnce.Do(func() {
		goose.SetBaseFS(migrations.FS)
		goose.SetLogger(goose.NopLogger())
		if err := goose.SetDialect("sqlite3"); err != nil {
			t.Fatalf("goose dialect: %v", err)
		}
	})
	configInitOnce.Do(func() {
		// Avoid relying on real .env or env vars during tests; just load defaults.
		// DB_ENCRYPTION_KEY is required by config.Load, so provide a dummy.
		if os.Getenv("DB_ENCRYPTION_KEY") == "" {
			os.Setenv("DB_ENCRYPTION_KEY", testHexKey)
		}
		_ = config.Load()
	})

	id := dbCounter.Add(1)
	dbPath := filepath.Join(t.TempDir(), fmt.Sprintf("test-%d.db", id))
	dsn := testDSN(dbPath)

	w, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open write: %v", err)
	}
	w.SetMaxOpenConns(1)
	if err := w.Ping(); err != nil {
		t.Fatalf("ping write: %v", err)
	}
	if err := goose.Up(w, "."); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	r, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open read: %v", err)
	}
	r.SetMaxOpenConns(4)
	if err := r.Ping(); err != nil {
		t.Fatalf("ping read: %v", err)
	}

	prevDB, prevRDB := store.DB, store.RDB
	store.DB, store.RDB = w, r

	// Wipe the response cache between tests so a previous test's GET hit
	// can't leak across the test boundary.
	mw.InvalidateCacheAll()

	t.Cleanup(func() {
		mw.InvalidateCacheAll()
		w.Close()
		r.Close()
		store.DB, store.RDB = prevDB, prevRDB
	})
}

// newTestServer returns the full Echo router under test. setupTestEnv must
// already have been called.
func newTestServer(t *testing.T) *echo.Echo {
	t.Helper()
	return CreateServer()
}

// HTTP helpers

// loginAdmin posts to /api/auth/login as the seeded `admin` user with the
// given password (which becomes the user's password on first login). Returns
// the bearer token.
func loginAdmin(t *testing.T, e *echo.Echo, password string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": password,
	})
	rec := doRequest(t, e, http.MethodPost, "/api/auth/login", body, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("login expected 200, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	var env struct {
		Success bool
		Data    struct {
			Token string `json:"token"`
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	if env.Data.Token == "" {
		t.Fatalf("no token returned: %s", rec.Body.String())
	}
	return env.Data.Token
}

// adminToken is a one-liner for the common case: setup env, login admin with
// a fresh password, return token.
func adminToken(t *testing.T, e *echo.Echo) string {
	t.Helper()
	return loginAdmin(t, e, "admin-pw")
}

// doRequest serves one HTTP request through the echo router and returns the
// recorded response. token is included as a Bearer header when non-empty.
func doRequest(t *testing.T, e *echo.Echo, method, path string, body []byte, token string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != nil {
		r = httptest.NewRequest(method, path, bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, r)
	return rec
}

// decodeEnvelope unmarshals a httpx.Response with a typed Data into out.
// Caller passes a pointer that out.Data should be unmarshaled into.
func decodeEnvelope(t *testing.T, body io.Reader, dataOut any) (errMsg string, success bool) {
	t.Helper()
	raw, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var env struct {
		Error   string
		Success bool
		Data    json.RawMessage
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode env: %v (body=%s)", err, string(raw))
	}
	if dataOut != nil && len(env.Data) > 0 && string(env.Data) != "null" {
		if err := json.Unmarshal(env.Data, dataOut); err != nil {
			t.Fatalf("decode data: %v (data=%s)", err, string(env.Data))
		}
	}
	return env.Error, env.Success
}

// asJSON marshals into bytes for POST/PUT bodies and fails the test on error.
func asJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// containsString asserts substr appears in the response body.
func containsString(t *testing.T, body, substr string) {
	t.Helper()
	if !strings.Contains(body, substr) {
		t.Errorf("body %q does not contain %q", body, substr)
	}
}

// countTableRows reads a row count for an arbitrary WHERE.
func countTableRows(t *testing.T, table, where string, args ...any) int {
	t.Helper()
	q := "SELECT COUNT(*) FROM " + table
	if where != "" {
		q += " WHERE " + where
	}
	var n int
	if err := store.RDB.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// fetchBalanceAmount loads just the amount field for a balance.
func fetchBalanceAmount(t *testing.T, id string) float64 {
	t.Helper()
	var a float64
	if err := store.RDB.QueryRow(`SELECT amount FROM balances WHERE id = ?`, id).Scan(&a); err != nil {
		t.Fatalf("fetch balance %s: %v", id, err)
	}
	return a
}

// getStoreDB returns the active write DB (post-setup). Useful when tests need
// to insert directly to bypass model validation or to set up a state that
// the public API can't produce.
func getStoreDB() *sql.DB {
	return store.DB
}

// approxEqualF compares floats with tolerance: REAL columns + json round-trip
// can introduce small noise.
func approxEqualF(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 1e-9
}
