package legacyimport

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"clinic-api/internal/database"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/vfs/adiantum"
)

// The fixtures in testdata/legacy are a small synthetic export with the
// columns the importer reads, including the edge cases it reports on.

// importDSN is a relative database URI with the fixed key of the throwaway
// test databases; the importer uses it as given.
func importDSN(name string) string {
	return "file:" + name + "?vfs=adiantum&hexkey=" + strings.Repeat("0", 62) + "ff" +
		"&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=recursive_triggers(1)"
}

// fixtureDir returns the absolute path of the fixtures, so tests can move to
// a temp working directory.
func fixtureDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("testdata", "legacy"))
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func openImported(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func count(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func text(t *testing.T, db *sql.DB, query string, args ...any) string {
	t.Helper()
	var s string
	if err := db.QueryRow(query, args...).Scan(&s); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return s
}

// copyFixtures copies the fixtures to a new folder for tests that change them.
func copyFixtures(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		raw, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dst
}

var importedTables = []string{
	"rooms", "product_categories", "products", "product_prices", "procedures", "procedure_prices",
	"patients", "balances", "appointments", "appointment_procedures", "invoices", "invoice_items",
	"balance_transactions",
}

func tableIDs(t *testing.T, db *sql.DB) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for _, table := range importedTables {
		rows, err := db.Query(`SELECT id FROM ` + table)
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				t.Fatal(err)
			}
			ids = append(ids, id)
		}
		rows.Close()
		sort.Strings(ids)
		out[table] = ids
	}
	return out
}

func TestRun_ImportsTheFixtures(t *testing.T) {
	in := fixtureDir(t)
	t.Chdir(t.TempDir())
	dsn := importDSN("clinic.db")
	if err := Run(Options{InputDir: in, DBPath: dsn, ReportPath: "report.txt"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	db := openImported(t, dsn)

	for query, want := range map[string]int{
		`SELECT COUNT(*) FROM patients`:                                                            8,
		`SELECT COUNT(*) FROM appointments`:                                                        11,
		`SELECT COUNT(*) FROM appointment_procedures`:                                              3,
		`SELECT COUNT(*) FROM invoices`:                                                            5,
		`SELECT COUNT(*) FROM invoice_items`:                                                       5,
		`SELECT COUNT(*) FROM balance_transactions`:                                                6,
		`SELECT COUNT(*) FROM balances WHERE entity_type = 'patient'`:                              6,
		`SELECT COUNT(*) FROM products`:                                                            3,
		`SELECT COUNT(*) FROM product_prices`:                                                      3,
		`SELECT COUNT(*) FROM product_categories`:                                                  1,
		`SELECT COUNT(*) FROM procedures WHERE name LIKE '% (deprecated)'`:                         5,
		`SELECT COUNT(*) FROM procedures WHERE name LIKE '% (deprecated)' AND is_active = 0`:       5,
		`SELECT COUNT(*) FROM procedure_prices WHERE procedure_id IN ('301', '302', '303', '306')`: 4,
		`SELECT COUNT(*) FROM rooms WHERE name IN ('Laser Suite', 'Unassigned')`:                   2,
	} {
		if got := count(t, db, query); got != want {
			t.Errorf("%s = %d, want %d", query, got, want)
		}
	}

	t.Run("patients", func(t *testing.T) {
		cases := []struct{ id, first, middle, last, gender, dob, blood, notes, created string }{
			{"101", "Alpha", "", "Tester", "Female", "1990-01-05", "A+", "First fixture patient", "2024-01-10T00:00:00Z"},
			{"102", "Charlie", "Q", "Sample", "Male", "2026-06-15", "O+", "", "2024-01-11T00:00:00Z"},
			{"103", "Delta", "", "Example", "", "", "AB+", "", "2024-01-01T00:00:00Z"},
			{"104", "Echo", "", "Dummy", "Female", "1927-06-15", "", "", "2024-01-12T00:00:00Z"},
			{"105", "Foxtrot", "", "Fixture", "Female", "", "B-", "", "2024-01-13T00:00:00Z"},
			{"999", "Hotel", "", "Missing", "", "", "", backfillNote, "2024-01-01T00:00:00Z"},
			{"106", "Golf", "", "Backfill", "", "", "", backfillNote, "2024-01-01T00:00:00Z"},
			{"998", "India", "", "Unknown", "", "", "", backfillNote, "2024-01-01T00:00:00Z"},
		}
		for _, c := range cases {
			var got [8]string
			if err := db.QueryRow(`SELECT first_name, middle_name, last_name, gender, date_of_birth, blood_type, notes, created_at FROM patients WHERE id = ?`, c.id).
				Scan(&got[0], &got[1], &got[2], &got[3], &got[4], &got[5], &got[6], &got[7]); err != nil {
				t.Fatalf("patient %s: %v", c.id, err)
			}
			want := [8]string{c.first, c.middle, c.last, c.gender, c.dob, c.blood, c.notes, c.created}
			if got != want {
				t.Errorf("patient %s = %q, want %q", c.id, got, want)
			}
		}
		var weight, height float64
		if err := db.QueryRow(`SELECT weight, height FROM patients WHERE id = '103'`).Scan(&weight, &height); err != nil {
			t.Fatal(err)
		}
		if weight != 0 || height != 170 {
			t.Errorf("patient 103 weight/height = %v/%v, want 0/170", weight, height)
		}
	})

	t.Run("appointment times convert from Beirut wall clock to UTC", func(t *testing.T) {
		cases := []struct{ id, start, end, status, room string }{
			{"201", "2026-03-28T08:00:00Z", "2026-03-28T08:30:00Z", "Completed", "Room 1"},
			{"202", "2026-03-30T07:00:00Z", "2026-03-30T07:30:00Z", "Completed", "Room 2"},
			// 00:30 on 29 March 2026 does not exist; it lands on 01:30, so the
			// 90-minute slot is stored as 30 minutes.
			{"203", "2026-03-28T22:30:00Z", "2026-03-28T23:00:00Z", "Completed", "Laser Suite"},
			// 23:30 on 25 October 2025 happens twice; the second pass is used.
			{"204", "2025-10-25T21:30:00Z", "2025-10-25T21:45:00Z", "Completed", "Unassigned"},
			{"205", "2025-10-24T07:00:00Z", "2025-10-24T08:00:00Z", "Completed", "Room 1"},
			{"206", "2025-10-27T08:00:00Z", "2025-10-27T09:00:00Z", "Cancelled", "Room 1"},
			{"207", "2099-01-10T07:00:00Z", "2099-01-10T07:30:00Z", "In-Progress", "Room 3"},
			{"208", "2099-01-10T08:00:00Z", "2099-01-10T08:30:00Z", "Scheduled", "Room 3"},
			{"209", "2099-01-10T09:00:00Z", "2099-01-10T09:30:00Z", "Cancelled", "Room 3"},
			{"210", "2099-01-10T10:00:00Z", "2099-01-10T10:30:00Z", "Scheduled", "Room 3"},
			{"213", "2026-02-12T09:00:00Z", "2026-02-12T09:30:00Z", "Completed", "Room 1"},
		}
		for _, c := range cases {
			var got [4]string
			if err := db.QueryRow(`SELECT a.start_time, a.end_time, a.status, r.name FROM appointments a JOIN rooms r ON r.id = a.room_id WHERE a.id = ?`, c.id).
				Scan(&got[0], &got[1], &got[2], &got[3]); err != nil {
				t.Fatalf("appointment %s: %v", c.id, err)
			}
			if want := [4]string{c.start, c.end, c.status, c.room}; got != want {
				t.Errorf("appointment %s = %q, want %q", c.id, got, want)
			}
		}
		notes := map[string]string{
			"201": "Procedure: Fixture Laser (11) | Before the spring change",
			"203": "Procedure: Fixture Peel (12) | Inside the skipped hour",
			"208": "Procedure: Fixture Laser (11)",
			"210": "Procedure: 99",
		}
		for id, want := range notes {
			if got := text(t, db, `SELECT notes FROM appointments WHERE id = ?`, id); got != want {
				t.Errorf("appointment %s notes = %q, want %q", id, got, want)
			}
		}
		links := map[string]string{"201": "Fixture Laser (deprecated)", "203": "Fixture Peel (deprecated)", "208": "Fixture Laser (deprecated)"}
		for id, want := range links {
			if got := text(t, db, `SELECT p.name FROM appointment_procedures ap JOIN procedures p ON p.id = ap.procedure_id WHERE ap.appointment_id = ?`, id); got != want {
				t.Errorf("appointment %s procedure = %q, want %q", id, got, want)
			}
		}
	})

	t.Run("catalog", func(t *testing.T) {
		for id, want := range map[string]string{
			"301": "Fixture Peel (deprecated)|Clinic Procedure|Legacy category: Skin|120",
			"302": "Fixture Surgery (deprecated)|Hospital Surgery|Legacy category: Surgery|2500",
			"303": "Fixture Minor (deprecated)|Minor Surgery||0",
			"306": "Fixture Other (deprecated)|||50",
		} {
			got := text(t, db, `SELECT p.name || '|' || COALESCE(t.name, '') || '|' || p.remarks || '|' || printf('%g', pp.price)
				FROM procedures p LEFT JOIN procedure_types t ON t.id = p.type_id JOIN procedure_prices pp ON pp.procedure_id = p.id WHERE p.id = ?`, id)
			if got != want {
				t.Errorf("procedure %s = %q, want %q", id, got, want)
			}
		}
		if got := text(t, db, `SELECT remarks FROM procedures WHERE name = 'Fixture Laser (deprecated)'`); got != "Legacy appointment category" {
			t.Errorf("category procedure remarks = %q", got)
		}
		for id, want := range map[string]string{"401": "Fixture Serum|12|45.5", "402": "Fixture Cream|0|20", "403": "Fixture Gel|3|0"} {
			got := text(t, db, `SELECT p.name || '|' || p.quantity || '|' || printf('%g', pp.price) FROM products p JOIN product_prices pp ON pp.product_id = p.id WHERE p.id = ?`, id)
			if got != want {
				t.Errorf("product %s = %q, want %q", id, got, want)
			}
		}
	})

	t.Run("invoices and ledger", func(t *testing.T) {
		for num, want := range map[int]string{
			5001: "200|20|180|USD|101",
			5002: "-50|0|-50|USD|999",
			5003: "0|0|0|USD|102",
			5004: "75|0|75|EUR|103",
			0:    "10|0|10|USD|104",
		} {
			got := text(t, db, `SELECT printf('%g|%g|%g', i.amount, i.discount_value, i.final_amount) || '|' || i.currency_id || '|' || b.entity_id
				FROM invoices i JOIN balances b ON b.id = i.to_balance_id WHERE i.invoice_number = ?`, num)
			if got != want {
				t.Errorf("invoice %d = %q, want %q", num, got, want)
			}
		}
		items := []string{}
		rows, err := db.Query(`SELECT it.item_type || '|' || COALESCE(p.name, it.item_id) || '|' || it.quantity || '|' || printf('%g', it.amount) || '|' || it.notes
			FROM invoice_items it LEFT JOIN procedures p ON p.id = it.item_id ORDER BY it.notes`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			items = append(items, s)
		}
		rows.Close()
		wantItems := []string{
			"other||1|0|Fixture Consult",
			"procedure|Fixture Peel (deprecated)|1|150|Fixture Peel",
			"product||2|25|Fixture Serum | Take home",
			"procedure|Fixture Laser (deprecated)|1|-50|Refund line",
			"procedure||1|40|Unmatched line",
		}
		if !reflect.DeepEqual(items, wantItems) {
			t.Errorf("invoice items =\n%s\nwant\n%s", strings.Join(items, "\n"), strings.Join(wantItems, "\n"))
		}

		for entity, want := range map[string]string{
			"self": "-47.5|145|0",
			"101":  "80|0|100",
			"102":  "-30|0|30",
			"103":  "-10|0|10",
			"104":  "12.5|0|0",
			"998":  "-5|0|5",
			"999":  "0|0|0",
		} {
			got := text(t, db, `SELECT printf('%g|%g|%g', amount, total_in, total_out) FROM balances WHERE entity_id = ?`, entity)
			if got != want {
				t.Errorf("balance of %s = %s, want %s", entity, got, want)
			}
		}
		if n := count(t, db, `SELECT COUNT(*) FROM balance_transactions WHERE transaction_type = 'charge'`); n != 2 {
			t.Errorf("charges = %d, want 2", n)
		}
		if n := count(t, db, `SELECT COUNT(*) FROM balance_transactions t JOIN invoices i ON i.id = t.source_id
			WHERE t.source_type = 'invoice' AND i.invoice_number = 5001`); n != 2 {
			t.Errorf("transactions linked to invoice 5001 = %d, want 2", n)
		}
	})

	t.Run("imported rows stay out of the sync log", func(t *testing.T) {
		if n := count(t, db, `SELECT COUNT(*) FROM sync_log`); n != 1 {
			t.Errorf("sync_log entries = %d, want only the seeded version row", n)
		}
		if n := count(t, db, `SELECT COUNT(*) FROM sync_log WHERE table_name != 'versions'`); n != 0 {
			t.Errorf("imported rows logged for sync: %d", n)
		}
		if n := count(t, db, `SELECT applying FROM _sync_applying WHERE rowid = 1`); n != 0 {
			t.Errorf("apply guard left at %d", n)
		}
	})

	t.Run("report", func(t *testing.T) {
		raw, err := os.ReadFile("report.txt")
		if err != nil {
			t.Fatal(err)
		}
		report := strings.ReplaceAll(string(raw), "\r\n", "\n")
		var table []string
		table = append(table, fmt.Sprintf("%-26s %8s %8s %8s", "table/source", "read", "written", "skipped"))
		for _, r := range []struct {
			title                string
			read, wrote, skipped int
		}{
			{"rooms (top-up)", 2, 2, 0},
			{"products", 3, 3, 0},
			{"procedures (deprecated)", 5, 5, 1},
			{"patients", 5, 8, 1},
			{"appointments", 13, 11, 2},
			{"invoices", 5, 5, 0},
			{"invoice_items", 6, 5, 1},
			{"balance_transactions", 6, 6, 0},
			{"balances", 6, 6, 0},
		} {
			table = append(table, fmt.Sprintf("%-26s %8d %8d %8d", r.title, r.read, r.wrote, r.skipped))
		}
		if !strings.Contains(report, "ROW COUNTS PER TABLE\n--------------------\n"+strings.Join(table, "\n")+"\n") {
			t.Errorf("report counts differ; report:\n%s", report)
		}
		summary := report[strings.Index(report, "INVALID / ADJUSTED VALUES (summary)"):strings.Index(report, "DETAIL BY TABLE")]
		if n := strings.Count(summary, "\n  - ["); n != 24 {
			t.Errorf("summary lists %d adjusted values, want 24:\n%s", n, summary)
		}
		for _, want := range []string{
			"[rooms (top-up)] 1 appointment room name(s) not in the seed were created as new 'General' rooms: [Laser Suite].",
			`[patients] patient "105": unparseable date_of_birth "31-Foo-90" (left empty).`,
			"[patients] 3 patient(s) referenced elsewhere were not in patients.csv and were created from their name with empty details.",
			"[appointments] 1 appointment(s) had an unrecognized status (treated as Completed if past, else Scheduled): [Mystery].",
			"[invoices] 1 invoice(s) had a non-numeric invoice number (defaulted to 0).",
			"[invoice_items] 1 invoice item(s) reference invoice numbers with no header in invoices.csv (skipped): [9999].",
			"[balance_transactions] 1 transaction(s) had an unrecognized type (treated as payment): [Odd Type].",
			"Past appointments (end time before now) -> Completed (6).",
			"Procedure lines linked to a procedure (by name, else by category): 2 of 3 (1 left with empty item_id).",
		} {
			if !strings.Contains(report, want) {
				t.Errorf("report lacks %q", want)
			}
		}
	})
}

const backfillNote = "Backfilled during migration from a reference in appointments/invoices/transactions; only the name was available."

func TestRun_IDsAreStableAcrossRuns(t *testing.T) {
	in := fixtureDir(t)
	t.Chdir(t.TempDir())
	var runs []map[string][]string
	for _, dir := range []string{"first", "second"} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		dsn := importDSN(dir + "/clinic.db")
		if err := Run(Options{InputDir: in, DBPath: dsn, ReportPath: filepath.Join(dir, "report.txt")}); err != nil {
			t.Fatalf("Run in %s: %v", dir, err)
		}
		runs = append(runs, tableIDs(t, openImported(t, dsn)))
	}
	for _, table := range importedTables {
		if len(runs[0][table]) == 0 {
			t.Errorf("%s is empty", table)
		}
		if !reflect.DeepEqual(runs[0][table], runs[1][table]) {
			t.Errorf("%s ids differ between runs", table)
		}
	}
}

func TestRun_RefusesADatabaseWithData(t *testing.T) {
	in := fixtureDir(t)
	t.Chdir(t.TempDir())
	dsn := importDSN("clinic.db")
	if err := Run(Options{InputDir: in, DBPath: dsn, ReportPath: "report.txt"}); err != nil {
		t.Fatal(err)
	}
	db := openImported(t, dsn)
	before := tableIDs(t, db)

	err := Run(Options{InputDir: in, DBPath: dsn, ReportPath: "again.txt"})
	want := "database already contains data (patients has 8 rows); import expects a freshly migrated DB. Re-run against an empty DB, or pass --force to override"
	if err == nil || err.Error() != want {
		t.Fatalf("second import: %v, want %q", err, want)
	}

	// Forcing the same export again fails on the first duplicate key.
	err = Run(Options{InputDir: in, DBPath: dsn, Force: true, ReportPath: "forced.txt"})
	if err == nil || !strings.HasPrefix(err.Error(), "insert into rooms: ") || !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("forced import: %v, want a duplicate key on rooms", err)
	}
	if after := tableIDs(t, db); !reflect.DeepEqual(after, before) {
		t.Error("a refused import changed the data")
	}
	for _, name := range []string{"again.txt", "forced.txt"} {
		if _, err := os.Stat(name); err == nil {
			t.Errorf("%s was written", name)
		}
	}
}

// With Backup, an encrypted snapshot of the database as it was before the
// import is taken next to it.
func TestRun_BackupHoldsThePreImportDatabase(t *testing.T) {
	in := fixtureDir(t)
	t.Chdir(t.TempDir())
	dsn := importDSN("clinic.db")
	if err := Run(Options{InputDir: in, DBPath: dsn, Backup: true, ReportPath: "report.txt"}); err != nil {
		t.Fatal(err)
	}
	backups, err := filepath.Glob(filepath.Join("backup", "clinic-*.db"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups = %v (%v), want one", backups, err)
	}
	snap, err := database.OpenStandalone(backups[0])
	if err != nil {
		t.Fatal(err)
	}
	defer snap.Close()
	if n := count(t, snap, `SELECT COUNT(*) FROM patients`); n != 0 {
		t.Errorf("backup holds %d patients, want the empty pre-import database", n)
	}
	if n := count(t, openImported(t, dsn), `SELECT COUNT(*) FROM patients`); n != 8 {
		t.Errorf("database holds %d patients after the import, want 8", n)
	}
}

// A failure writes nothing: the whole import is one transaction, and missing
// files are found before it starts.
func TestRun_FailureWritesNothing(t *testing.T) {
	src := fixtureDir(t)
	t.Chdir(t.TempDir())
	dsn := importDSN("clinic.db")

	// An invoice without a patient has no balance to point to.
	broken := copyFixtures(t, src)
	f, err := os.OpenFile(filepath.Join(broken, "invoices.csv"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("5005,10,0,10,USD,21-Mar-26,,Walk In,No patient on file\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	err = Run(Options{InputDir: broken, DBPath: dsn, ReportPath: "report.txt"})
	if err == nil || !strings.HasPrefix(err.Error(), "insert into invoices: ") || !strings.Contains(err.Error(), "FOREIGN KEY") {
		t.Fatalf("import with an invoice without patient: %v, want a foreign key failure", err)
	}

	missing := copyFixtures(t, src)
	if err := os.Remove(filepath.Join(missing, "transactions.csv")); err != nil {
		t.Fatal(err)
	}
	err = Run(Options{InputDir: missing, DBPath: dsn, ReportPath: "report.txt"})
	if err == nil || !strings.HasPrefix(err.Error(), "read ") || !strings.Contains(err.Error(), "transactions.csv") {
		t.Fatalf("import without transactions.csv: %v", err)
	}

	db := openImported(t, dsn)
	for _, table := range []string{"patients", "appointments", "appointment_procedures", "invoices", "invoice_items", "balance_transactions", "products", "product_categories"} {
		if n := count(t, db, `SELECT COUNT(*) FROM `+table); n != 0 {
			t.Errorf("%s has %d rows after failed imports", table, n)
		}
	}
	if n := count(t, db, `SELECT COUNT(*) FROM rooms`); n != 8 {
		t.Errorf("rooms = %d, want the 8 seeded", n)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM procedures WHERE name LIKE '% (deprecated)'`); n != 0 {
		t.Errorf("deprecated procedures = %d after failed imports", n)
	}
	if n := count(t, db, `SELECT applying FROM _sync_applying WHERE rowid = 1`); n != 0 {
		t.Errorf("apply guard left at %d", n)
	}
	if _, err := os.Stat("report.txt"); err == nil {
		t.Error("a report was written for a failed import")
	}
}

// The report is written after the commit; when it can't be written the
// import stays in place but Run still returns an error.
func TestRun_ReportFailureKeepsTheImport(t *testing.T) {
	in := fixtureDir(t)
	t.Chdir(t.TempDir())
	dsn := importDSN("clinic.db")
	if err := os.Mkdir("report-folder", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := Run(Options{InputDir: in, DBPath: dsn, ReportPath: "report-folder"}); err == nil {
		t.Fatal("Run succeeded although the report path is a folder")
	}
	if n := count(t, openImported(t, dsn), `SELECT COUNT(*) FROM patients`); n != 8 {
		t.Errorf("patients = %d, want the committed import (8)", n)
	}
}
