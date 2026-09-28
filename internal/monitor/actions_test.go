package monitor

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"clinic-api/internal/config"
	"clinic-api/internal/database"
	"clinic-api/internal/database/store"
	"clinic-api/internal/pdf"
	"clinic-api/internal/realtime"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/vfs/adiantum"
)

// setupMonitorDB moves to a temp working directory and installs a migrated
// test database, at the build's default path, as the store pools.
func setupMonitorDB(t *testing.T) (db *sql.DB, path string) {
	t.Helper()
	t.Chdir(t.TempDir())
	path, err := filepath.Abs(filepath.Join(config.DataDir(), "clinic.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	dsn := "file:" + filepath.ToSlash(path) +
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
	return w, path
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func queryInt(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return n
}

// addUsers adds one more active user and an inactive one next to the seeded
// super-admin and returns the ids of the active users.
func addUsers(t *testing.T, db *sql.DB) []string {
	t.Helper()
	mustExec(t, db, `INSERT INTO users (id, username, display_name, role, is_active) VALUES ('user-on', 'user-on', 'Active User', 'staff', 1)`)
	mustExec(t, db, `INSERT INTO users (id, username, display_name, role, is_active) VALUES ('user-off', 'user-off', 'Inactive User', 'staff', 0)`)
	rows, err := db.Query(`SELECT id FROM users WHERE is_active = 1 ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if len(ids) != 2 {
		t.Fatalf("active users = %v, want the super-admin and user-on", ids)
	}
	return ids
}

func TestExpireDiscounts(t *testing.T) {
	db, _ := setupMonitorDB(t)
	const stamp = "2026-01-01T00:00:00Z"
	add := func(id string, active int, endDate any) {
		mustExec(t, db, `INSERT INTO discounts (id, name, discount_type, value, is_active, end_date, created_at, updated_at)
			VALUES (?, ?, 'offer', 10, ?, ?, ?, ?)`, id, "Offer "+id, active, endDate, stamp, stamp)
	}
	today := string(store.ClinicToday())
	add("ended", 1, "2000-01-01")
	add("ended-stamp", 1, "2000-01-01T10:00:00Z")
	add("ends-today", 1, today)
	add("ends-later", 1, "2999-12-31")
	add("no-end", 1, "")
	add("null-end", 1, nil)
	add("already-off", 0, "2000-01-01")
	logged := queryInt(t, db, `SELECT COALESCE(MAX(seq), 0) FROM sync_log`)

	if err := ExpireDiscounts(); err != nil {
		t.Fatal(err)
	}
	sameDay := string(store.ClinicToday()) == today

	for id, wantActive := range map[string]int{
		"ended": 0, "ended-stamp": 0, "ends-today": 1, "ends-later": 1, "no-end": 1, "null-end": 1, "already-off": 0,
	} {
		if id == "ends-today" && !sameDay {
			continue
		}
		var active int
		var updated string
		if err := db.QueryRow(`SELECT is_active, updated_at FROM discounts WHERE id = ?`, id).Scan(&active, &updated); err != nil {
			t.Fatal(err)
		}
		changed := id == "ended" || id == "ended-stamp"
		if active != wantActive || (updated != stamp) != changed {
			t.Errorf("%s: is_active %d updated_at %s, want is_active %d and updated_at changed = %v", id, active, updated, wantActive, changed)
		}
	}
	if n := queryInt(t, db, `SELECT COUNT(*) FROM sync_log WHERE table_name = 'discounts' AND op = 'update' AND seq > ?`, logged); n != 2 {
		t.Errorf("sync_log update entries for expired discounts = %d, want 2", n)
	}
}

// Each scheduled appointment starting within 30 minutes gets one reminder
// per active user, once.
func TestSendAppointmentReminders_OncePerAppointment(t *testing.T) {
	db, _ := setupMonitorDB(t)
	active := addUsers(t, db)
	mustExec(t, db, `INSERT INTO rooms (id, name, type) VALUES ('room-1', 'Reminder Room', 'General')`)
	mustExec(t, db, `INSERT INTO patients (id, first_name, last_name, date_of_birth, contact) VALUES ('patient-1', 'Alpha', 'Tester', '1990-01-01', '555-0101')`)
	now := time.Now().UTC()
	at := func(d time.Duration) string { return now.Add(d).Format(time.RFC3339) }
	add := func(id, status string, start time.Duration) {
		mustExec(t, db, `INSERT INTO appointments (id, patient_id, room_id, start_time, end_time, status) VALUES (?, 'patient-1', 'room-1', ?, ?, ?)`,
			id, at(start), at(start+30*time.Minute), status)
	}
	add("soon", "Scheduled", 10*time.Minute)
	add("sooner", "Scheduled", 20*time.Minute)
	add("later", "Scheduled", 40*time.Minute)
	add("started", "Scheduled", -5*time.Minute)
	add("cancelled", "Cancelled", 10*time.Minute)
	add("in-progress", "In-Progress", 10*time.Minute)

	for run := 0; run < 2; run++ {
		if err := SendAppointmentReminders(); err != nil {
			t.Fatal(err)
		}
	}

	beirut, err := time.LoadLocation("Asia/Beirut")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		id    string
		start time.Duration
	}{{"soon", 10 * time.Minute}, {"sooner", 20 * time.Minute}} {
		start, _ := time.Parse(time.RFC3339, at(c.start))
		wantDesc := "Alpha Tester at " + start.In(beirut).Format("15:04")
		for _, uid := range active {
			n := queryInt(t, db, `SELECT COUNT(*) FROM notifications WHERE user_id = ? AND action = ? AND title = 'Upcoming appointment in 30 minutes' AND description = ?`,
				uid, "appointment-reminder:"+c.id, wantDesc)
			if n != 1 {
				t.Errorf("reminders for %s to %s = %d, want 1 (%q)", c.id, uid, n, wantDesc)
			}
		}
	}
	if n := queryInt(t, db, `SELECT COUNT(*) FROM notifications`); n != 2*len(active) {
		t.Errorf("notifications = %d, want %d", n, 2*len(active))
	}
	if n := queryInt(t, db, `SELECT COUNT(*) FROM notifications WHERE user_id = 'user-off'`); n != 0 {
		t.Errorf("inactive user got %d reminders", n)
	}
}

func TestCleanupExpiredTokens(t *testing.T) {
	db, _ := setupMonitorDB(t)
	var admin string
	if err := db.QueryRow(`SELECT id FROM users WHERE username = 'super-admin'`).Scan(&admin); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	tokens := map[string]string{
		"long-gone":    "2000-01-01T00:00:00Z",
		"just-expired": now.Add(-time.Minute).Format(time.RFC3339),
		"still-valid":  now.Add(time.Minute).Format(time.RFC3339),
		"far-future":   "2999-01-01T00:00:00Z",
	}
	for id, exp := range tokens {
		mustExec(t, db, `INSERT INTO tokens (id, token, user_id, expires_at) VALUES (?, ?, ?, ?)`, id, "token-"+id, admin, exp)
	}
	if err := CleanupExpiredTokens(); err != nil {
		t.Fatal(err)
	}
	for id, keep := range map[string]bool{"long-gone": false, "just-expired": false, "still-valid": true, "far-future": true} {
		if got := queryInt(t, db, `SELECT COUNT(*) FROM tokens WHERE id = ?`, id) == 1; got != keep {
			t.Errorf("token %s kept = %v, want %v", id, got, keep)
		}
	}
}

// A product at or below its threshold gets one notice per active user; the
// notices go away when stock recovers and come back on the next drop.
func TestCheckLowStock_OneNoticePerProduct(t *testing.T) {
	db, _ := setupMonitorDB(t)
	active := addUsers(t, db)
	for _, p := range []struct {
		id, name      string
		quantity, min int
	}{{"low", "Low Serum", 2, 5}, {"edge", "Edge Cream", 5, 5}, {"fine", "Fine Gel", 9, 5}} {
		mustExec(t, db, `INSERT INTO products (id, name, quantity, min_threshold) VALUES (?, ?, ?, ?)`, p.id, p.name, p.quantity, p.min)
	}
	notices := func(id string) int {
		return queryInt(t, db, `SELECT COUNT(*) FROM notifications WHERE action = ?`, "low-stock:"+id)
	}
	check := func(ids ...string) {
		CheckLowStock(ids)
		WaitAsync()
	}

	check("low", "edge", "fine", "low", "", "missing")
	check("low", "edge")
	for _, c := range []struct {
		id, title, desc string
	}{
		{"low", "Low stock: Low Serum", "Quantity 2 at or below min threshold 5."},
		{"edge", "Low stock: Edge Cream", "Quantity 5 at or below min threshold 5."},
	} {
		for _, uid := range active {
			n := queryInt(t, db, `SELECT COUNT(*) FROM notifications WHERE user_id = ? AND action = ? AND title = ? AND description = ?`,
				uid, "low-stock:"+c.id, c.title, c.desc)
			if n != 1 {
				t.Errorf("notices for %s to %s = %d, want 1", c.id, uid, n)
			}
		}
	}
	if n := notices("fine"); n != 0 {
		t.Errorf("notices for a product above its threshold = %d", n)
	}

	mustExec(t, db, `UPDATE products SET quantity = 20 WHERE id = 'low'`)
	check("low")
	if n := notices("low"); n != 0 {
		t.Errorf("notices after the stock recovered = %d, want 0", n)
	}
	if n := notices("edge"); n != len(active) {
		t.Errorf("notices of another product = %d, want %d", n, len(active))
	}

	mustExec(t, db, `UPDATE products SET quantity = 1 WHERE id = 'low'`)
	check("low")
	if n := notices("low"); n != len(active) {
		t.Errorf("notices after a new drop = %d, want %d", n, len(active))
	}
}

// Clearing a recovered product's notices tells each account that lost one,
// once, that its notifications changed; accounts that held none hear nothing,
// and a product still low sends nothing either.
func TestCheckLowStock_ClearTellsTheAccountsThatLostANotice(t *testing.T) {
	db, _ := setupMonitorDB(t)
	active := addUsers(t, db)
	mustExec(t, db, `INSERT INTO products (id, name, quantity, min_threshold) VALUES ('low', 'Low Serum', 2, 5), ('still-low', 'Still Low Gel', 1, 5)`)
	CheckLowStock([]string{"low", "still-low"})
	WaitAsync()
	// Activated after the notices went out, so it holds none of them.
	mustExec(t, db, `INSERT INTO users (id, username, display_name, role, is_active) VALUES ('user-late', 'user-late', 'Late User', 'staff', 1)`)

	clients := map[string]*realtime.Client{}
	for _, uid := range append(slices.Clone(active), "user-off", "user-late") {
		c := realtime.Register(uid)
		t.Cleanup(c.Close)
		clients[uid] = c
	}
	mustExec(t, db, `UPDATE products SET quantity = 20 WHERE id = 'low'`)
	CheckLowStock([]string{"low", "still-low"})
	WaitAsync()

	for uid, c := range clients {
		var got []string
		for len(c.Events()) > 0 {
			got = append(got, (<-c.Events()).Type)
		}
		var want []string
		if slices.Contains(active, uid) {
			want = []string{realtime.NotificationsChanged}
		}
		if !slices.Equal(got, want) {
			t.Errorf("events to %s = %v, want %v", uid, got, want)
		}
	}
}

// A backup is skipped while the database files are unchanged, and only the
// 24 newest clinic-*.db snapshots are kept.
func TestBackupDatabase_SkipsUnchangedAndKeeps24(t *testing.T) {
	_, dbPath := setupMonitorDB(t)
	prev := lastBackupMod
	lastBackupMod = time.Time{}
	t.Cleanup(func() { lastBackupMod = prev })

	dir := config.BackupDir()
	if err := os.MkdirAll(filepath.Join(dir, "clinic-folder.db"), 0o700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30; i++ {
		name := fmt.Sprintf("clinic-20000101T0000%02d.000000000Z.db", i)
		if err := os.WriteFile(filepath.Join(dir, name), []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"notes.txt", "clinic-keep.txt", "other.db"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	snapshots := func() []string {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, e := range entries {
			if !e.IsDir() && strings.HasPrefix(e.Name(), "clinic-") && strings.HasSuffix(e.Name(), ".db") {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)
		return names
	}

	if err := BackupDatabase(); err != nil {
		t.Fatal(err)
	}
	first := snapshots()
	if len(first) != 24 || first[0] != "clinic-20000101T000007.000000000Z.db" || strings.HasPrefix(first[23], "clinic-2000") {
		t.Fatalf("after the first backup: %d snapshots from %s to %s", len(first), first[0], first[len(first)-1])
	}

	if err := BackupDatabase(); err != nil {
		t.Fatal(err)
	}
	if again := snapshots(); strings.Join(again, ",") != strings.Join(first, ",") {
		t.Fatalf("a backup ran although nothing changed: %v", again)
	}

	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(dbPath, later, later); err != nil {
		t.Fatal(err)
	}
	if err := BackupDatabase(); err != nil {
		t.Fatal(err)
	}
	third := snapshots()
	if len(third) != 24 || third[0] != "clinic-20000101T000008.000000000Z.db" || third[23] == first[23] {
		t.Fatalf("after a change: %d snapshots from %s to %s", len(third), third[0], third[len(third)-1])
	}
	for _, name := range []string{"notes.txt", "clinic-keep.txt", "other.db", "clinic-folder.db"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s was removed: %v", name, err)
		}
	}
}

// Cached PDFs older than 15 minutes are removed; other files stay.
func TestCleanupPDFCache(t *testing.T) {
	t.Chdir(t.TempDir())
	dir := pdf.TmpDir()
	old := time.Now().Add(-20 * time.Minute)
	files := map[string]bool{"old.pdf": false, "fresh.pdf": true, "old.txt": true}
	for name := range files {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("%PDF-"), 0o600); err != nil {
			t.Fatal(err)
		}
		if name != "fresh.pdf" {
			if err := os.Chtimes(p, old, old); err != nil {
				t.Fatal(err)
			}
		}
	}
	folder := filepath.Join(dir, "old-folder.pdf")
	if err := os.Mkdir(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(folder, old, old); err != nil {
		t.Fatal(err)
	}

	if err := CleanupPDFCache(); err != nil {
		t.Fatal(err)
	}
	for name, keep := range files {
		_, err := os.Stat(filepath.Join(dir, name))
		if (err == nil) != keep {
			t.Errorf("%s kept = %v, want %v", name, err == nil, keep)
		}
	}
	if _, err := os.Stat(folder); err != nil {
		t.Errorf("a folder named like a PDF was removed: %v", err)
	}
}
