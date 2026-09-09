package sync_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	stdsync "sync"
	"testing"

	"clinic-api/internal/database/migrations"
	syncpkg "clinic-api/internal/sync"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/vfs/adiantum"
	"github.com/pressly/goose/v3"
)

// testKey is the fixed encryption key of the throwaway node databases.
var testKey = strings.Repeat("0", 62) + "ff"

var gooseOnce stdsync.Once

func nodeDSN(path string) string {
	return "file:" + filepath.ToSlash(path) +
		"?vfs=adiantum" +
		"&hexkey=" + testKey +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(1)" +
		"&_pragma=recursive_triggers(1)"
}

// newNode opens a freshly migrated node database with a single connection,
// like the write pool of the app.
func newNode(t *testing.T) *sql.DB {
	t.Helper()
	gooseOnce.Do(func() {
		goose.SetBaseFS(migrations.FS)
		goose.SetLogger(goose.NopLogger())
		if err := goose.SetDialect("sqlite3"); err != nil {
			t.Fatalf("goose dialect: %v", err)
		}
	})
	db, err := sql.Open("sqlite3", nodeDSN(filepath.Join(t.TempDir(), "node.db")))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if err := goose.Up(db, "."); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// newNodePair returns two nodes that both hold the fixture rows: peer
// produces changes, node applies them.
func newNodePair(t *testing.T) (peer, node *sql.DB) {
	t.Helper()
	peer, node = newNode(t), newNode(t)
	loadFixture(t, peer)
	loadFixture(t, node)
	return peer, node
}

const fixtureTime = "2026-01-01T00:00:00Z"

// fixtureRows holds one row for every synced table (id "<table>-1", or
// "fixture-role" for roles) plus leaf rows ("-2") that nothing references,
// so they can be deleted. Parents come first.
var fixtureRows = []struct{ table, id, insert string }{
	{"roles", "fixture-role", `INSERT INTO roles (name, label, scopes) VALUES ('fixture-role', 'Fixture Role', '')`},
	{"currencies", "currencies-1", `INSERT INTO currencies (id, code, name, symbol, exchange_rate, created_at, updated_at) VALUES ('currencies-1', 'TSA', 'Test Currency A', 'A', 1, $T, $T)`},
	{"currencies", "currencies-2", `INSERT INTO currencies (id, code, name, symbol, exchange_rate, created_at, updated_at) VALUES ('currencies-2', 'TSB', 'Test Currency B', 'B', 1, $T, $T)`},
	{"allergies", "allergies-1", `INSERT INTO allergies (id, name, description, created_at) VALUES ('allergies-1', 'Fixture Allergy', '', $T)`},
	{"medicines", "medicines-1", `INSERT INTO medicines (id, name, description, created_at) VALUES ('medicines-1', 'Fixture Medicine', '', $T)`},
	{"procedure_types", "procedure_types-1", `INSERT INTO procedure_types (id, name, description, created_at) VALUES ('procedure_types-1', 'Fixture Type', '', $T)`},
	{"procedure_categories", "procedure_categories-1", `INSERT INTO procedure_categories (id, name, description, parent_id, created_at) VALUES ('procedure_categories-1', 'Fixture Category', '', '', $T)`},
	{"rooms", "rooms-1", `INSERT INTO rooms (id, name, type, is_available, created_at) VALUES ('rooms-1', 'Fixture Room', 'General', 1, $T)`},
	{"rooms", "rooms-2", `INSERT INTO rooms (id, name, type, is_available, created_at) VALUES ('rooms-2', 'Fixture Room Two', 'Consultation', 1, $T)`},
	{"product_categories", "product_categories-1", `INSERT INTO product_categories (id, name, description, parent_id, created_at) VALUES ('product_categories-1', 'Fixture Products', '', '', $T)`},
	{"countries", "countries-1", `INSERT INTO countries (id, name) VALUES ('countries-1', 'Fixture Country')`},
	{"lebanon_cities", "lebanon_cities-1", `INSERT INTO lebanon_cities (id, name, governorate, district) VALUES ('lebanon_cities-1', 'Fixture City', '', '')`},
	{"expenses", "expenses-1", `INSERT INTO expenses (id, name, notes, created_at, updated_at) VALUES ('expenses-1', 'Fixture Expense', '', $T, $T)`},
	{"expenses", "expenses-2", `INSERT INTO expenses (id, name, notes, created_at, updated_at) VALUES ('expenses-2', 'Fixture Expense Two', '', $T, $T)`},
	{"suppliers", "suppliers-1", `INSERT INTO suppliers (id, name, contact, email, address, notes, created_at, updated_at) VALUES ('suppliers-1', 'Fixture Supplier', '', 'supplier@example.com', '', '', $T, $T)`},
	{"suppliers", "suppliers-2", `INSERT INTO suppliers (id, name, contact, email, address, notes, created_at, updated_at) VALUES ('suppliers-2', 'Fixture Supplier Two', '', '', '', '', $T, $T)`},
	{"versions", "versions-1", `INSERT INTO versions (id, version, platform, url, sha256, notes, created_at) VALUES ('versions-1', '9.9.1', 'linux', 'https://example.com/one.zip', '', '', $T)`},
	{"versions", "versions-2", `INSERT INTO versions (id, version, platform, url, sha256, notes, created_at) VALUES ('versions-2', '9.9.2', 'windows', 'https://example.com/two.zip', '', '', $T)`},
	{"users", "users-1", `INSERT INTO users (id, username, password_hash, display_name, role, is_active, created_at, updated_at) VALUES ('users-1', 'fixture-user', '', 'Fixture User', 'staff', 1, $T, $T)`},
	{"users", "users-2", `INSERT INTO users (id, username, password_hash, display_name, role, is_active, created_at, updated_at) VALUES ('users-2', 'fixture-user-two', '', 'Fixture User Two', 'nurse', 1, $T, $T)`},
	{"procedures", "procedures-1", `INSERT INTO procedures (id, name, type_id, category_id, price_note, is_active, remarks, includes, created_at, updated_at) VALUES ('procedures-1', 'Fixture Procedure', 'procedure_types-1', 'procedure_categories-1', '', 1, '', '[]', $T, $T)`},
	{"procedures", "procedures-2", `INSERT INTO procedures (id, name, type_id, category_id, price_note, is_active, remarks, includes, created_at, updated_at) VALUES ('procedures-2', 'Fixture Procedure Two', '', '', '', 1, '', '[]', $T, $T)`},
	{"products", "products-1", `INSERT INTO products (id, name, category_id, quantity, min_threshold, created_at) VALUES ('products-1', 'Fixture Product', 'product_categories-1', 5, 1, $T)`},
	{"discounts", "discounts-1", `INSERT INTO discounts (id, name, description, discount_type, value_type, value, is_active, created_at, updated_at) VALUES ('discounts-1', 'Fixture Offer', '', 'offer', 'percentage', 10, 1, $T, $T)`},
	{"discounts", "discounts-2", `INSERT INTO discounts (id, name, description, discount_type, value_type, value, is_active, created_at, updated_at) VALUES ('discounts-2', 'Fixture Offer Two', '', 'offer', 'fixed', 5, 1, $T, $T)`},
	{"employees", "employees-1", `INSERT INTO employees (id, user_id, first_name, last_name, role, contact, email, date_of_birth, employment_type, created_at, updated_at) VALUES ('employees-1', 'users-1', 'Fixture', 'Employee', 'Nurse', '555-0100', 'employee@example.com', '1990-01-01', 'Full-time', $T, $T)`},
	{"employees", "employees-2", `INSERT INTO employees (id, user_id, first_name, last_name, role, contact, email, date_of_birth, employment_type, created_at, updated_at) VALUES ('employees-2', '', 'Fixture', 'Employee Two', 'Receptionist', '555-0102', '', '', 'Part-time', $T, $T)`},
	{"patients", "patients-1", `INSERT INTO patients (id, first_name, last_name, date_of_birth, contact, notes, created_at, updated_at) VALUES ('patients-1', 'Fixture', 'Patient', '1990-01-01', '555-0101', '', $T, $T)`},
	{"patients", "patients-2", `INSERT INTO patients (id, first_name, last_name, date_of_birth, contact, notes, created_at, updated_at) VALUES ('patients-2', 'Fixture', 'Patient Two', '1991-02-02', '555-0103', '', $T, $T)`},
	{"balances", "balances-1", `INSERT INTO balances (id, entity_type, entity_id, entity_name, currency_id, amount, total_in, total_out, created_at, updated_at) VALUES ('balances-1', 'patient', 'patients-1', 'Fixture Patient', 'USD', 0, 0, 0, $T, $T)`},
	{"balances", "balances-1b", `INSERT INTO balances (id, entity_type, entity_id, entity_name, currency_id, amount, total_in, total_out, created_at, updated_at) VALUES ('balances-1b', 'supplier', 'suppliers-1', 'Fixture Supplier', 'USD', 0, 0, 0, $T, $T)`},
	{"balances", "balances-2", `INSERT INTO balances (id, entity_type, entity_id, entity_name, currency_id, amount, total_in, total_out, created_at, updated_at) VALUES ('balances-2', 'expense', 'expenses-1', 'Fixture Expense', 'USD', 0, 0, 0, $T, $T)`},
	{"appointments", "appointments-1", `INSERT INTO appointments (id, patient_id, room_id, start_time, end_time, status, notes, created_at, updated_at) VALUES ('appointments-1', 'patients-1', 'rooms-1', '2026-01-05T08:00:00Z', '2026-01-05T09:00:00Z', 'Scheduled', '', $T, $T)`},
	{"appointments", "appointments-2", `INSERT INTO appointments (id, patient_id, room_id, start_time, end_time, status, notes, created_at, updated_at) VALUES ('appointments-2', 'patients-1', 'rooms-1', '2026-01-06T08:00:00Z', '2026-01-06T09:00:00Z', 'Completed', '', $T, $T)`},
	{"prescriptions", "prescriptions-1", `INSERT INTO prescriptions (id, patient_id, prescribed_by_id, start_date, end_date, created_at, updated_at) VALUES ('prescriptions-1', 'patients-1', 'employees-1', '2026-01-05', '', $T, $T)`},
	{"prescriptions", "prescriptions-2", `INSERT INTO prescriptions (id, patient_id, prescribed_by_id, start_date, end_date, created_at, updated_at) VALUES ('prescriptions-2', 'patients-1', 'employees-1', '2026-01-06', '', $T, $T)`},
	{"employee_schedules", "employee_schedules-1", `INSERT INTO employee_schedules (id, employee_id, day_of_week, start_time, end_time, start_date, end_date, is_active, created_at, updated_at) VALUES ('employee_schedules-1', 'employees-1', 1, '09:00', '17:00', '2026-01-01', '', 1, $T, $T)`},
	{"employee_schedules", "employee_schedules-2", `INSERT INTO employee_schedules (id, employee_id, day_of_week, start_time, end_time, start_date, end_date, is_active, created_at, updated_at) VALUES ('employee_schedules-2', 'employees-1', 2, '09:00', '17:00', '2026-01-01', '', 1, $T, $T)`},
	{"holidays", "holidays-1", `INSERT INTO holidays (id, name, start_date, end_date, notes, created_by, created_at, updated_at) VALUES ('holidays-1', 'Fixture Holiday', '2026-02-01', '2026-02-01', '', '', $T, $T)`},
	{"holidays", "holidays-2", `INSERT INTO holidays (id, name, start_date, end_date, notes, created_by, created_at, updated_at) VALUES ('holidays-2', 'Fixture Holiday Two', '2026-03-01', '2026-03-02', '', '', $T, $T)`},
	{"employee_schedule_changes", "employee_schedule_changes-1", `INSERT INTO employee_schedule_changes (id, employee_id, type, start_date, end_date, start_time, end_time, status, notes, created_at, updated_at) VALUES ('employee_schedule_changes-1', 'employees-1', 'timeoff', '2026-02-02', '2026-02-02', '', '', 'pending', '', $T, $T)`},
	{"employee_schedule_changes", "employee_schedule_changes-2", `INSERT INTO employee_schedule_changes (id, employee_id, type, start_date, end_date, start_time, end_time, status, notes, created_at, updated_at) VALUES ('employee_schedule_changes-2', 'employees-1', 'overtime', '2026-02-03', '2026-02-03', '18:00', '20:00', 'accepted', '', $T, $T)`},
	{"employee_salaries", "employee_salaries-1", `INSERT INTO employee_salaries (id, employee_id, amount, currency_id, is_active, effective_date, notes, created_at, updated_at) VALUES ('employee_salaries-1', 'employees-1', 1000, 'USD', 1, '2026-01-01', '', $T, $T)`},
	{"employee_salaries", "employee_salaries-2", `INSERT INTO employee_salaries (id, employee_id, amount, currency_id, is_active, effective_date, notes, created_at, updated_at) VALUES ('employee_salaries-2', 'employees-1', 900, 'USD', 0, '2025-01-01', '', $T, $T)`},
	{"employee_salary_preparations", "employee_salary_preparations-1", `INSERT INTO employee_salary_preparations (id, employee_id, period_start, period_end, salary_id, transaction_id, currency_id, base_salary, adjustment, prepared_amount, notes, created_by, created_at) VALUES ('employee_salary_preparations-1', 'employees-1', '2026-01-01', '2026-01-31', 'employee_salaries-1', '', 'USD', 1000, 0, 1000, '', '', $T)`},
	{"patient_allergies", "patient_allergies-1", `INSERT INTO patient_allergies (id, patient_id, allergy_id, notes, created_at) VALUES ('patient_allergies-1', 'patients-1', 'allergies-1', '', $T)`},
	{"patient_medicines", "patient_medicines-1", `INSERT INTO patient_medicines (id, patient_id, medicine_id, is_active, notes, created_at) VALUES ('patient_medicines-1', 'patients-1', 'medicines-1', 1, '', $T)`},
	{"procedure_allergy_conflicts", "procedure_allergy_conflicts-1", `INSERT INTO procedure_allergy_conflicts (id, procedure_id, allergy_id, notes, created_at) VALUES ('procedure_allergy_conflicts-1', 'procedures-1', 'allergies-1', '', $T)`},
	{"procedure_prices", "procedure_prices-1", `INSERT INTO procedure_prices (id, procedure_id, price, is_active, created_at) VALUES ('procedure_prices-1', 'procedures-1', 100, 1, $T)`},
	{"product_prices", "product_prices-1", `INSERT INTO product_prices (id, product_id, price, is_active, created_at) VALUES ('product_prices-1', 'products-1', 20, 1, $T)`},
	{"product_allergy_conflicts", "product_allergy_conflicts-1", `INSERT INTO product_allergy_conflicts (id, product_id, allergy_id, notes, created_at) VALUES ('product_allergy_conflicts-1', 'products-1', 'allergies-1', '', $T)`},
	{"appointment_procedures", "appointment_procedures-1", `INSERT INTO appointment_procedures (id, patient_id, procedure_id, appointment_id, assigned_to_id, notes, created_at, updated_at) VALUES ('appointment_procedures-1', 'patients-1', 'procedures-1', 'appointments-1', 'employees-1', '', $T, $T)`},
	{"appointment_procedures", "appointment_procedures-2", `INSERT INTO appointment_procedures (id, patient_id, procedure_id, appointment_id, assigned_to_id, notes, created_at, updated_at) VALUES ('appointment_procedures-2', 'patients-1', 'procedures-1', 'appointments-1', NULL, '', $T, $T)`},
	{"prescription_medicines", "prescription_medicines-1", `INSERT INTO prescription_medicines (id, medicine_id, prescription_id, instructions, created_at) VALUES ('prescription_medicines-1', 'medicines-1', 'prescriptions-1', '', $T)`},
	{"balance_transactions", "balance_transactions-1", `INSERT INTO balance_transactions (id, from_balance_id, to_balance_id, amount, currency_id, transaction_type, transaction_method, source_type, source_id, description, created_by, created_at, voided_at) VALUES ('balance_transactions-1', 'balances-1', 'balances-1b', 50, 'USD', 'payment', 'cash', '', '', '', '', $T, '')`},
	{"balance_transactions", "balance_transactions-2", `INSERT INTO balance_transactions (id, from_balance_id, to_balance_id, amount, currency_id, transaction_type, transaction_method, source_type, source_id, description, created_by, created_at, voided_at) VALUES ('balance_transactions-2', 'balances-1', 'balances-1b', 30, 'USD', 'payment', 'card', '', '', '', '', $T, '')`},
	{"invoices", "invoices-1", `INSERT INTO invoices (id, invoice_number, from_balance_id, to_balance_id, amount, discount_id, discount_value, final_amount, currency_id, notes, created_by, created_at, updated_at, voided_at) VALUES ('invoices-1', 1, 'balances-1b', 'balances-1', 100, '', 0, 100, 'USD', '', '', $T, $T, '')`},
	{"invoices", "invoices-2", `INSERT INTO invoices (id, invoice_number, from_balance_id, to_balance_id, amount, discount_id, discount_value, final_amount, currency_id, notes, created_by, created_at, updated_at, voided_at) VALUES ('invoices-2', 2, 'balances-1b', 'balances-1', 40, '', 0, 40, 'USD', '', '', $T, $T, '')`},
	{"invoice_items", "invoice_items-1", `INSERT INTO invoice_items (id, invoice_id, item_type, item_id, quantity, amount, final_amount, notes, created_at) VALUES ('invoice_items-1', 'invoices-1', 'procedure', 'procedures-1', 1, 100, 100, '', $T)`},
	{"invoice_items", "invoice_items-2", `INSERT INTO invoice_items (id, invoice_id, item_type, item_id, quantity, amount, final_amount, notes, created_at) VALUES ('invoice_items-2', 'invoices-1', 'product', 'products-1', 2, 40, 40, '', $T)`},
}

// markColumn is a text column of each synced table that tests change to tell
// a local version of a row from a remote one.
var markColumn = map[string]string{
	"roles":                        "label",
	"currencies":                   "name",
	"allergies":                    "description",
	"medicines":                    "description",
	"procedure_types":              "description",
	"procedure_categories":         "description",
	"rooms":                        "name",
	"product_categories":           "description",
	"countries":                    "name",
	"lebanon_cities":               "district",
	"expenses":                     "notes",
	"suppliers":                    "notes",
	"versions":                     "notes",
	"users":                        "display_name",
	"procedures":                   "remarks",
	"products":                     "name",
	"discounts":                    "description",
	"employees":                    "email",
	"patients":                     "notes",
	"balances":                     "entity_name",
	"appointments":                 "notes",
	"prescriptions":                "end_date",
	"employee_schedules":           "end_date",
	"holidays":                     "notes",
	"employee_schedule_changes":    "notes",
	"employee_salaries":            "notes",
	"employee_salary_preparations": "notes",
	"patient_allergies":            "notes",
	"patient_medicines":            "notes",
	"procedure_allergy_conflicts":  "notes",
	"procedure_prices":             "created_at",
	"product_prices":               "created_at",
	"product_allergy_conflicts":    "notes",
	"appointment_procedures":       "notes",
	"prescription_medicines":       "instructions",
	"balance_transactions":         "description",
	"invoices":                     "notes",
	"invoice_items":                "notes",
}

func loadFixture(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, r := range fixtureRows {
		if _, err := db.Exec(strings.ReplaceAll(r.insert, "$T", "'"+fixtureTime+"'")); err != nil {
			t.Fatalf("fixture %s/%s: %v", r.table, r.id, err)
		}
	}
}

// baseRowID is the fixture row of table that other rows may reference.
func baseRowID(table string) string {
	if table == "roles" {
		return "fixture-role"
	}
	return table + "-1"
}

// leafRowID is the fixture row of table that nothing references.
func leafRowID(table string) string { return table + "-2" }

// tablesWithUpdatedAt returns the synced tables that carry updated_at, or
// the ones that don't.
func tablesWithUpdatedAt(want bool) []syncpkg.TableInfo {
	var out []syncpkg.TableInfo
	for _, ti := range syncpkg.SyncedTables {
		if ti.HasUpdatedAt == want {
			out = append(out, ti)
		}
	}
	return out
}

// setMark changes the mark column of one row and, when the table has it,
// its updated_at.
func setMark(t *testing.T, db *sql.DB, table, id, value, updatedAt string) {
	t.Helper()
	ti, ok := syncpkg.IsSyncedTable(table)
	if !ok {
		t.Fatalf("%s is not synced", table)
	}
	q := fmt.Sprintf(`UPDATE %q SET %q = ?`, table, markColumn[table])
	args := []any{value}
	if ti.HasUpdatedAt {
		q += `, updated_at = ?`
		args = append(args, updatedAt)
	}
	q += fmt.Sprintf(` WHERE %q = ?`, ti.PK())
	res, err := db.Exec(q, append(args, id)...)
	if err != nil {
		t.Fatalf("set mark %s/%s: %v", table, id, err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		t.Fatalf("set mark %s/%s: %d rows changed", table, id, n)
	}
}

// markOf returns the mark column of one row; ok is false when it is missing.
func markOf(t *testing.T, db *sql.DB, table, id string) (value string, ok bool) {
	t.Helper()
	ti, _ := syncpkg.IsSyncedTable(table)
	err := db.QueryRow(fmt.Sprintf(`SELECT %q FROM %q WHERE %q = ?`, markColumn[table], table, ti.PK()), id).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false
	}
	if err != nil {
		t.Fatalf("read mark %s/%s: %v", table, id, err)
	}
	return value, true
}

func rowExists(t *testing.T, db *sql.DB, table, id string) bool {
	t.Helper()
	_, ok := markOf(t, db, table, id)
	return ok
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func count(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", query, err)
	}
	return n
}

func maxSeq(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	n, err := syncpkg.MaxLogSeq(db)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// outgoing returns what db ships for its changes after seq: the sync_log
// entries enriched with the live rows, as the pull handler and push build them.
func outgoing(t *testing.T, db *sql.DB, after int64) []syncpkg.LogEntry {
	t.Helper()
	batch, err := syncpkg.LoadBatch(db, after, 10000)
	if err != nil {
		t.Fatalf("load batch: %v", err)
	}
	if err := syncpkg.EnrichBatch(db, batch); err != nil {
		t.Fatalf("enrich batch: %v", err)
	}
	return batch
}

func mustApply(t *testing.T, db *sql.DB, batch []syncpkg.LogEntry) (int64, []syncpkg.ConflictEntry) {
	t.Helper()
	applied, conflicts, err := syncpkg.Apply(db, batch)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	return applied, conflicts
}

func lastSeq(batch []syncpkg.LogEntry) int64 {
	var m int64
	for _, e := range batch {
		if e.Seq > m {
			m = e.Seq
		}
	}
	return m
}

// applyGuard is the sync apply flag the triggers consult.
func applyGuard(t *testing.T, db *sql.DB) int {
	t.Helper()
	return count(t, db, `SELECT applying FROM _sync_applying WHERE rowid = 1`)
}

// jsonField decodes one field of a row JSON document as a string.
func jsonField(t *testing.T, doc, field string) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(doc), &m); err != nil {
		t.Fatalf("decode %q: %v", doc, err)
	}
	s, _ := m[field].(string)
	return s
}
