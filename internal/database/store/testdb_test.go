package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"clinic-api/internal/database/migrations"

	_ "github.com/mattn/go-sqlite3"
	"github.com/pressly/goose/v3"
)

// dbCounter ensures each test gets a unique on-disk file path so they don't
// share state, even though we keep them around for the test process.
var dbCounter atomic.Uint64

var setupGooseOnce sync.Once

// setupTestDB opens a fresh SQLite database in t.TempDir, runs all migrations,
// and replaces the package-level store.DB / store.RDB handles for the duration
// of the test. It also clears all data between tests by using a unique file.
func setupTestDB(t *testing.T) {
	t.Helper()

	setupGooseOnce.Do(func() {
		goose.SetBaseFS(migrations.FS)
		goose.SetLogger(goose.NopLogger())
		if err := goose.SetDialect("sqlite3"); err != nil {
			t.Fatalf("goose dialect: %v", err)
		}
	})

	id := dbCounter.Add(1)
	dbPath := filepath.Join(t.TempDir(), fmt.Sprintf("test-%d.db", id))
	dsn := dbPath + "?_foreign_keys=1&_journal_mode=WAL&_busy_timeout=5000"

	w, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open write db: %v", err)
	}
	w.SetMaxOpenConns(1)
	if err := w.Ping(); err != nil {
		t.Fatalf("ping write db: %v", err)
	}
	if err := goose.Up(w, "."); err != nil {
		t.Fatalf("goose up: %v", err)
	}

	r, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open read db: %v", err)
	}
	r.SetMaxOpenConns(4)
	if err := r.Ping(); err != nil {
		t.Fatalf("ping read db: %v", err)
	}

	prevDB, prevRDB := DB, RDB
	DB, RDB = w, r

	t.Cleanup(func() {
		w.Close()
		r.Close()
		DB, RDB = prevDB, prevRDB
	})
}

// seedCurrency inserts a currency directly and returns the stored model.
func seedCurrency(t *testing.T, code, name, symbol string, rate float64) Currency {
	t.Helper()
	c := Currency{Code: code, Name: name, Symbol: symbol, ExchangeRate: rate}
	if err := c.Create(); err != nil {
		t.Fatalf("seedCurrency: %v", err)
	}
	return c
}

// seedSelfBalance creates the self-side balance for the given currency.
// (The migrations seed a self balance for the seeded currencies; for currencies
// we create here, callers may need to create a self balance explicitly.)
func seedSelfBalance(t *testing.T, currencyID, name string) Balance {
	t.Helper()
	id := "self"
	b := Balance{
		EntityType: "self",
		EntityID:   &id,
		EntityName: name,
		CurrencyID: currencyID,
	}
	if err := b.GetOrCreate(); err != nil {
		t.Fatalf("seedSelfBalance: %v", err)
	}
	return b
}

// seededCurrency returns one of the currencies seeded by the migrations.
// Returns the first currency by code.
func seededCurrency(t *testing.T) Currency {
	t.Helper()
	var c Currency
	if err := c.ScanRow(RDB.QueryRow(`SELECT ` + currencyColumns + ` FROM currencies ORDER BY code LIMIT 1`)); err != nil {
		t.Fatalf("no seeded currency found: %v", err)
	}
	return c
}

// seededSelfBalance returns the self balance seeded by the migrations for the
// given currency.
func seededSelfBalance(t *testing.T, currencyID string) Balance {
	t.Helper()
	var b Balance
	if err := b.ScanRow(RDB.QueryRow(`SELECT `+balanceColumns+` FROM balances WHERE entity_type = 'self' AND currency_id = ?`, currencyID)); err != nil {
		t.Fatalf("no seeded self balance for currency %s: %v", currencyID, err)
	}
	return b
}

// makePatient creates a minimal valid patient (no balances are created here;
// the Patient.Create method does that automatically).
func makePatient(t *testing.T, first, last, phone string) Patient {
	t.Helper()
	p := Patient{
		FirstName:   first,
		LastName:    last,
		Gender:      "Male",
		DateOfBirth: Date("1990-01-01"),
		Contact:     phone,
	}
	if err := p.Create(); err != nil {
		t.Fatalf("makePatient: %v", err)
	}
	return p
}

// patientBalance returns the patient balance row for the given patient and
// currency (created automatically by Patient.Create).
func patientBalance(t *testing.T, patientID, currencyID string) Balance {
	t.Helper()
	var b Balance
	err := b.ScanRow(RDB.QueryRow(`SELECT `+balanceColumns+` FROM balances WHERE entity_type='patient' AND entity_id=? AND currency_id=?`, patientID, currencyID))
	if err != nil {
		t.Fatalf("patientBalance: %v", err)
	}
	return b
}

// expenseBalance creates an expense balance (one-off) and returns it.
func expenseBalance(t *testing.T, name, currencyID string) Balance {
	t.Helper()
	id := "expense-" + name
	b := Balance{
		EntityType: "expense",
		EntityID:   &id,
		EntityName: name,
		CurrencyID: currencyID,
	}
	if err := b.GetOrCreate(); err != nil {
		t.Fatalf("expenseBalance: %v", err)
	}
	return b
}

// fetchBalance reloads a balance by ID, useful for checking amount changes
// after a transaction.
func fetchBalance(t *testing.T, id string) Balance {
	t.Helper()
	var b Balance
	if err := b.ScanRow(RDB.QueryRow(`SELECT ` + balanceColumns + ` FROM balances WHERE id = ?`, id)); err != nil {
		t.Fatalf("fetchBalance(%s): %v", id, err)
	}
	return b
}

// countRows is a tiny helper for assertions like "no transactions remain".
func countRows(t *testing.T, table, where string, args ...any) int {
	t.Helper()
	q := "SELECT COUNT(*) FROM " + table
	if where != "" {
		q += " WHERE " + where
	}
	var n int
	if err := RDB.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("countRows %s: %v", table, err)
	}
	return n
}

// approxEqual returns true if two floats are within a small epsilon. SQLite
// REAL stores doubles, but float math invites rounding noise in tests.
func approxEqual(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 1e-9
}
