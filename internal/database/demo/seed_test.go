package demo_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"clinic-api/internal/api/handlers"
	"clinic-api/internal/auth"
	"clinic-api/internal/config"
	"clinic-api/internal/database"
	"clinic-api/internal/database/store"
	syncpkg "clinic-api/internal/sync"
	"clinic-api/internal/validation"

	"github.com/labstack/echo/v4"
	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/vfs/adiantum"
)

// Demo data is not deterministic, so this is a smoke test only: it seeds a
// fresh database, checks the seed runs once, fills the main tables, logs
// every row it writes for sync, that the demo accounts can sign in, and that
// its entries and notices belong to those accounts the way the app's do.
func TestSeedDemo_Smoke(t *testing.T) {
	cfg := config.Load()
	cfg.PeerURL = ""
	cfg.SyncSecret = "test-sync-secret"
	cfg.PublishSecret = "test-publish-secret"
	cfg.PublicURL = "http://127.0.0.1:8080"
	cfg.JWTSecret = "test-jwt-secret"

	db := openTestDB(t)
	before := syncedKeys(t, db)

	if err := database.SeedDemo(db); err != nil {
		t.Fatalf("SeedDemo: %v", err)
	}
	after := tableCounts(t, db)
	if err := database.SeedDemo(db); err != nil {
		t.Fatalf("second SeedDemo: %v", err)
	}
	if again := tableCounts(t, db); fmt.Sprint(again) != fmt.Sprint(after) {
		t.Errorf("a second seed changed the data:\n%v\n%v", after, again)
	}
	if n := scalar(t, db, `SELECT COUNT(*) FROM goose_demo_version WHERE version_id = 1 AND is_applied = 1`); n != 1 {
		t.Errorf("demo version rows = %d, want 1", n)
	}

	for _, table := range []string{
		"allergies", "medicines", "product_categories", "products", "product_prices",
		"employees", "employee_salaries", "employee_schedules", "employee_schedule_changes", "holidays",
		"suppliers", "expenses", "patients", "patient_allergies", "patient_medicines",
		"prescriptions", "prescription_medicines", "discounts", "appointments", "appointment_procedures",
		"invoices", "invoice_items", "balances", "balance_transactions", "notifications",
	} {
		if after[table] == 0 {
			t.Errorf("%s is empty after the demo seed", table)
		}
	}

	t.Run("every row is logged for sync", func(t *testing.T) {
		now := syncedKeys(t, db)
		added := 0
		for _, ti := range syncpkg.SyncedTables {
			logged := map[string]bool{}
			rows, err := db.Query(`SELECT row_id FROM sync_log WHERE table_name = ?`, ti.Name)
			if err != nil {
				t.Fatal(err)
			}
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					t.Fatal(err)
				}
				logged[id] = true
			}
			rows.Close()
			missing := 0
			for id := range now[ti.Name] {
				if before[ti.Name][id] {
					continue
				}
				added++
				if !logged[id] {
					missing++
				}
			}
			if missing > 0 {
				t.Errorf("%s: %d demo rows are not in sync_log", ti.Name, missing)
			}
		}
		if added < 1000 {
			t.Errorf("the demo added %d synced rows, expected a large dataset", added)
		}
		if n := scalar(t, db, `SELECT COUNT(*) FROM sync_log WHERE table_name = 'balances' AND op = 'update' AND row_id IN (SELECT id FROM balances WHERE entity_type = 'self')`); n != 1 {
			t.Errorf("the recomputed clinic balance was logged %d times, want once as an update", n)
		}
		synced := map[string]bool{}
		for _, ti := range syncpkg.SyncedTables {
			synced[ti.Name] = true
		}
		rows, err := db.Query(`SELECT DISTINCT table_name FROM sync_log`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				t.Fatal(err)
			}
			if !synced[name] {
				t.Errorf("sync_log holds entries for %s, which is not synced", name)
			}
		}
	})

	t.Run("demo accounts sign in", func(t *testing.T) {
		rows, err := db.Query(`SELECT username, role FROM users WHERE username != 'super-admin' ORDER BY username`)
		if err != nil {
			t.Fatal(err)
		}
		type account struct{ username, role string }
		var accounts []account
		for rows.Next() {
			var a account
			if err := rows.Scan(&a.username, &a.role); err != nil {
				t.Fatal(err)
			}
			accounts = append(accounts, a)
		}
		rows.Close()
		if len(accounts) != 4 {
			t.Fatalf("demo accounts = %d, want 4", len(accounts))
		}
		roles := map[string]int{}
		for _, a := range accounts {
			roles[a.role]++
			token := signIn(t, a.username, "demo123")
			claims, err := auth.ParseToken(token)
			if err != nil {
				t.Errorf("%s: token does not parse: %v", a.username, err)
				continue
			}
			if claims["role"] != a.role {
				t.Errorf("%s: token role %v, want %s", a.username, claims["role"], a.role)
			}
		}
		if roles["admin"] != 1 || roles["nurse"] != 2 || roles["staff"] != 1 {
			t.Errorf("demo roles = %v, want 1 admin, 2 nurses, 1 staff", roles)
		}
		if code := loginStatus(t, accounts[0].username, "wrong-password"); code != http.StatusUnauthorized {
			t.Errorf("sign-in with a wrong password = %d, want 401", code)
		}
	})

	t.Run("entries name the demo account that made them", func(t *testing.T) {
		// The app writes the signed-in account's display name to created_by.
		names := map[string]bool{}
		rows, err := db.Query(`SELECT display_name FROM users WHERE username != 'super-admin'`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				t.Fatal(err)
			}
			names[name] = true
		}
		rows.Close()
		for _, table := range []string{"invoices", "holidays", "balance_transactions"} {
			rows, err := db.Query(`SELECT DISTINCT created_by FROM ` + table)
			if err != nil {
				t.Fatalf("%s: %v", table, err)
			}
			for rows.Next() {
				var by string
				if err := rows.Scan(&by); err != nil {
					t.Fatal(err)
				}
				if !names[by] {
					t.Errorf("%s: created_by %q is not a demo account's display name", table, by)
				}
			}
			rows.Close()
		}
	})

	t.Run("gift card names match their values", func(t *testing.T) {
		rows, err := db.Query(`SELECT name, value FROM discounts WHERE discount_type = 'gift'`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			var value float64
			if err := rows.Scan(&name, &value); err != nil {
				t.Fatal(err)
			}
			if want := fmt.Sprintf("$%g", value); !strings.HasSuffix(name, want) {
				t.Errorf("gift card %q is worth %s", name, want)
			}
		}
	})

	t.Run("every active account gets the notices", func(t *testing.T) {
		// Like the monitor's notices, the seeded ones go to every active account.
		if n := scalar(t, db, `SELECT COUNT(*) FROM users u WHERE u.is_active = 1
			AND NOT EXISTS (SELECT 1 FROM notifications n WHERE n.user_id = u.id)`); n != 0 {
			t.Errorf("%d active accounts have no notification", n)
		}
	})

	t.Run("the low-stock notice is worded from the seeded stock", func(t *testing.T) {
		// The monitor's wording, for a product the seed left at or below its
		// threshold; none when no product sits that low.
		type lowProduct struct {
			name          string
			quantity, min int
		}
		low := map[string]lowProduct{}
		rows, err := db.Query(`SELECT id, name, quantity, min_threshold FROM products WHERE quantity <= min_threshold`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var id string
			var p lowProduct
			if err := rows.Scan(&id, &p.name, &p.quantity, &p.min); err != nil {
				t.Fatal(err)
			}
			low[id] = p
		}
		rows.Close()

		notices, actions := 0, map[string]bool{}
		rows, err = db.Query(`SELECT action, title, description FROM notifications WHERE title LIKE 'Low stock: %'`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var action, title, description string
			if err := rows.Scan(&action, &title, &description); err != nil {
				t.Fatal(err)
			}
			notices++
			actions[action] = true
			id := strings.TrimPrefix(action, "low-stock:")
			p, ok := low[id]
			if !ok {
				t.Errorf("notice %q (action %q) does not name a product at or below its threshold", title, action)
				continue
			}
			want := store.LowStockNotice(id, p.name, p.quantity, p.min)
			if title != want.Title || description != want.Description {
				t.Errorf("notice %q %q is not the monitor's wording %q %q", title, description, want.Title, want.Description)
			}
		}
		rows.Close()

		if len(low) == 0 && notices > 0 {
			t.Errorf("%d low-stock notices exist but no product is at or below its threshold", notices)
		}
		if len(low) > 0 {
			if len(actions) != 1 {
				t.Errorf("the seed worded %d distinct low-stock notices, want the one product it picked", len(actions))
			}
			if n := scalar(t, db, `SELECT COUNT(*) FROM users u WHERE u.is_active = 1
				AND NOT EXISTS (SELECT 1 FROM notifications n WHERE n.user_id = u.id AND n.title LIKE 'Low stock: %')`); n != 0 {
				t.Errorf("%d active accounts lack the low-stock notice", n)
			}
		}
	})

	t.Run("fictional identities and phone formats", func(t *testing.T) {
		for _, q := range []struct {
			table string
			query string
		}{
			{"employees", `SELECT email FROM employees WHERE email != ''`},
			{"patients", `SELECT email FROM patients WHERE email != ''`},
			{"suppliers", `SELECT email FROM suppliers WHERE email != ''`},
		} {
			rows, err := db.Query(q.query)
			if err != nil {
				t.Fatalf("%s: %v", q.table, err)
			}
			for rows.Next() {
				var email string
				if err := rows.Scan(&email); err != nil {
					t.Fatal(err)
				}
				if !strings.HasSuffix(email, "@example.com") &&
					!strings.HasSuffix(email, "@example.org") &&
					!strings.HasSuffix(email, "@example.net") {
					t.Errorf("%s: email %q does not end with @example.(com|org|net)", q.table, email)
				}
			}
			rows.Close()
		}

		phoneRe := regexp.MustCompile(`^\+1 555 01\d{2}$`)
		for _, q := range []struct {
			table string
			query string
		}{
			{"employees", `SELECT contact FROM employees WHERE contact != ''`},
			{"patients", `SELECT contact FROM patients WHERE contact != ''`},
			{"suppliers", `SELECT contact FROM suppliers WHERE contact != ''`},
		} {
			rows, err := db.Query(q.query)
			if err != nil {
				t.Fatalf("%s: %v", q.table, err)
			}
			for rows.Next() {
				var contact string
				if err := rows.Scan(&contact); err != nil {
					t.Fatal(err)
				}
				if !phoneRe.MatchString(contact) {
					t.Errorf("%s: contact %q does not match +1 555 01xx", q.table, contact)
				}
				if msg := validation.Phone(contact); msg != "" {
					t.Errorf("%s: contact %q failed validation.Phone: %s", q.table, contact, msg)
				}
			}
			rows.Close()
		}

		const consultID = "6df0a12d-18ae-48a7-8d26-fb8fa70f4a31"
		var consultCount int
		if err := db.QueryRow(`SELECT COUNT(*) FROM appointment_procedures WHERE procedure_id = ?`, consultID).Scan(&consultCount); err != nil {
			t.Fatalf("consultation procedure query: %v", err)
		}
		if consultCount == 0 {
			t.Errorf("expected appointment with procedure %s, found none", consultID)
		}
	})
}

// openTestDB installs a freshly migrated test database as the store pools.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := "file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "demo.db")) +
		"?vfs=adiantum&hexkey=" + strings.Repeat("0", 62) + "ff" +
		"&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=recursive_triggers(1)"
	w, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatal(err)
	}
	w.SetMaxOpenConns(1)
	if err := database.Migrate(w); err != nil {
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

func scalar(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// syncedKeys returns the primary keys of every synced table.
func syncedKeys(t *testing.T, db *sql.DB) map[string]map[string]bool {
	t.Helper()
	out := map[string]map[string]bool{}
	for _, ti := range syncpkg.SyncedTables {
		rows, err := db.Query(fmt.Sprintf(`SELECT %q FROM %q`, ti.PK(), ti.Name))
		if err != nil {
			t.Fatal(err)
		}
		keys := map[string]bool{}
		for rows.Next() {
			var k string
			if err := rows.Scan(&k); err != nil {
				t.Fatal(err)
			}
			keys[k] = true
		}
		rows.Close()
		out[ti.Name] = keys
	}
	return out
}

func tableCounts(t *testing.T, db *sql.DB) map[string]int {
	t.Helper()
	counts := map[string]int{}
	for _, ti := range syncpkg.SyncedTables {
		counts[ti.Name] = scalar(t, db, fmt.Sprintf(`SELECT COUNT(*) FROM %q`, ti.Name))
	}
	for _, table := range []string{"notifications", "sync_log"} {
		counts[table] = scalar(t, db, `SELECT COUNT(*) FROM `+table)
	}
	return counts
}

func loginRequest(t *testing.T, username, password string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(string(body)))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	if err := handlers.Login(echo.New().NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}
	return rec
}

func loginStatus(t *testing.T, username, password string) int {
	t.Helper()
	return loginRequest(t, username, password).Code
}

func signIn(t *testing.T, username, password string) string {
	t.Helper()
	rec := loginRequest(t, username, password)
	var env struct {
		Success bool
		Data    struct {
			Token string `json:"token"`
		}
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &env) != nil || !env.Success || env.Data.Token == "" {
		t.Fatalf("sign-in as %s = %d %s", username, rec.Code, rec.Body.String())
	}
	return env.Data.Token
}
