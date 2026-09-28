package sync_test

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"clinic-api/internal/database/store"
	syncpkg "clinic-api/internal/sync"
)

// Deletes of invoices, invoice lines, ledger transactions and versions are
// refused on both builds and recorded as no_delete conflicts.
func TestApply_NoDeleteTablesRefuseDeletes(t *testing.T) {
	peer, node := newNodePair(t)
	since := maxSeq(t, peer)
	for _, table := range []string{"versions", "balance_transactions", "invoice_items", "invoices"} {
		mustExec(t, peer, `DELETE FROM `+table+` WHERE id = ?`, leafRowID(table))
	}
	batch := outgoing(t, peer, since)
	if len(batch) != 4 {
		t.Fatalf("outgoing batch has %d entries, want 4", len(batch))
	}
	for _, e := range batch {
		if e.Op != "delete" || e.RowJSON != nil || e.UpdatedAt != "" {
			t.Fatalf("delete entry = %+v, want op delete without row data", e)
		}
	}

	applied, conflicts := mustApply(t, node, batch)
	if applied != lastSeq(batch) {
		t.Errorf("applied seq = %d, want %d (refused rows count as handled)", applied, lastSeq(batch))
	}
	var order []string
	for _, c := range conflicts {
		order = append(order, c.Table)
		if c.Resolution != "no_delete" || c.RemoteJSON != "" || c.RowID != leafRowID(c.Table) {
			t.Errorf("conflict = %+v, want no_delete for %s with no remote row", c, leafRowID(c.Table))
		}
		if got := jsonField(t, c.LocalJSON, "id"); got != c.RowID {
			t.Errorf("conflict local row id = %q, want %q", got, c.RowID)
		}
	}
	if want := []string{"invoice_items", "invoices", "balance_transactions", "versions"}; !reflect.DeepEqual(order, want) {
		t.Errorf("conflict order = %v, want children first %v", order, want)
	}
	for _, table := range []string{"versions", "balance_transactions", "invoice_items", "invoices"} {
		if !rowExists(t, node, table, leafRowID(table)) {
			t.Errorf("%s/%s was deleted", table, leafRowID(table))
		}
	}
	if n := count(t, node, `SELECT COUNT(*) FROM sync_conflicts WHERE resolution = 'no_delete' AND remote_json = ''`); n != 4 {
		t.Errorf("no_delete rows in sync_conflicts = %d, want 4", n)
	}
}

// Applied rows are written with the apply guard raised, so they never enter
// the receiver's own outbox; the guard is lowered again afterwards.
func TestApply_WritesSilentlyAndLowersTheGuard(t *testing.T) {
	peer, node := newNodePair(t)
	since := maxSeq(t, peer)
	setMark(t, peer, "patients", "patients-1", "remote", "2026-02-01T00:00:00Z")
	mustExec(t, peer, `INSERT INTO rooms (id, name, type) VALUES ('rooms-9', 'Peer Room', 'General')`)
	mustExec(t, peer, `DELETE FROM rooms WHERE id = 'rooms-2'`)
	batch := outgoing(t, peer, since)

	before := maxSeq(t, node)
	logged := count(t, node, `SELECT COUNT(*) FROM sync_log`)
	mustApply(t, node, batch)
	if got := maxSeq(t, node); got != before {
		t.Errorf("node outbox moved from seq %d to %d during apply", before, got)
	}
	if got := count(t, node, `SELECT COUNT(*) FROM sync_log`); got != logged {
		t.Errorf("node outbox has %d entries after apply, want %d", got, logged)
	}
	if g := applyGuard(t, node); g != 0 {
		t.Fatalf("apply guard = %d after apply, want 0", g)
	}
	if !rowExists(t, node, "rooms", "rooms-9") || rowExists(t, node, "rooms", "rooms-2") {
		t.Fatal("batch was not applied")
	}

	mustExec(t, node, `INSERT INTO rooms (id, name, type) VALUES ('rooms-10', 'Node Room', 'General')`)
	if n := count(t, node, `SELECT COUNT(*) FROM sync_log WHERE table_name = 'rooms' AND row_id = 'rooms-10'`); n != 1 {
		t.Errorf("a local write after apply was logged %d times, want 1", n)
	}
}

// Upserts are applied parents first and deletes children first, whatever
// the order of the batch.
func TestApply_OrdersParentsBeforeChildren(t *testing.T) {
	peer, node := newNodePair(t)
	since := maxSeq(t, peer)
	mustExec(t, peer, `INSERT INTO rooms (id, name, type) VALUES ('rooms-9', 'Peer Room', 'General')`)
	mustExec(t, peer, `INSERT INTO patients (id, first_name, last_name, date_of_birth, contact) VALUES ('patients-9', 'Peer', 'Patient', '1990-01-01', '555-0109')`)
	mustExec(t, peer, `INSERT INTO appointments (id, patient_id, room_id, start_time, end_time, status) VALUES ('appointments-9', 'patients-9', 'rooms-9', '2026-01-09T08:00:00Z', '2026-01-09T09:00:00Z', 'Scheduled')`)
	mustExec(t, peer, `INSERT INTO appointment_procedures (id, patient_id, procedure_id, appointment_id) VALUES ('appointment_procedures-9', 'patients-9', 'procedures-1', 'appointments-9')`)
	inserts := outgoing(t, peer, since)
	sort.Slice(inserts, func(i, j int) bool { return inserts[i].Seq > inserts[j].Seq })
	mustApply(t, node, inserts)
	for _, r := range [][2]string{{"rooms", "rooms-9"}, {"patients", "patients-9"}, {"appointments", "appointments-9"}, {"appointment_procedures", "appointment_procedures-9"}} {
		if !rowExists(t, node, r[0], r[1]) {
			t.Errorf("%s/%s missing after a children-first batch", r[0], r[1])
		}
	}

	since = maxSeq(t, peer)
	mustExec(t, peer, `DELETE FROM appointment_procedures WHERE id = 'appointment_procedures-9'`)
	mustExec(t, peer, `DELETE FROM appointments WHERE id = 'appointments-9'`)
	mustExec(t, peer, `DELETE FROM patients WHERE id = 'patients-9'`)
	mustExec(t, peer, `DELETE FROM rooms WHERE id = 'rooms-9'`)
	deletes := outgoing(t, peer, since)
	sort.Slice(deletes, func(i, j int) bool { return deletes[i].Seq > deletes[j].Seq })
	mustApply(t, node, deletes)
	for _, r := range [][2]string{{"rooms", "rooms-9"}, {"patients", "patients-9"}, {"appointments", "appointments-9"}, {"appointment_procedures", "appointment_procedures-9"}} {
		if rowExists(t, node, r[0], r[1]) {
			t.Errorf("%s/%s still present after a parents-first delete batch", r[0], r[1])
		}
	}
}

// With the ledger hook installed (as the server does at startup), applying
// transactions rebuilds the cached totals of both balances they touch from
// the receiver's full ledger, without logging the recomputed balances.
func TestApply_RecomputesBalancesOfAppliedTransactions(t *testing.T) {
	prev := syncpkg.RecalcBalance
	syncpkg.RecalcBalance = store.RecalculateBalanceWithTx
	t.Cleanup(func() { syncpkg.RecalcBalance = prev })

	peer, node := newNodePair(t)
	since := maxSeq(t, peer)
	mustExec(t, peer, `INSERT INTO balance_transactions (id, from_balance_id, to_balance_id, amount, transaction_type, transaction_method) VALUES ('balance_transactions-9', 'balances-1', 'balances-1b', 20, 'payment', 'cash')`)
	batch := outgoing(t, peer, since)

	logged := count(t, node, `SELECT COUNT(*) FROM sync_log`)
	mustApply(t, node, batch)

	want := map[string][3]float64{
		"balances-1":  {-100, 0, 100},
		"balances-1b": {100, 100, 0},
		"balances-2":  {0, 0, 0},
	}
	for id, w := range want {
		var got [3]float64
		if err := node.QueryRow(`SELECT amount, total_in, total_out FROM balances WHERE id = ?`, id).Scan(&got[0], &got[1], &got[2]); err != nil {
			t.Fatal(err)
		}
		if got != w {
			t.Errorf("%s amount/in/out = %v, want %v", id, got, w)
		}
	}
	if got := count(t, node, `SELECT COUNT(*) FROM sync_log`); got != logged {
		t.Errorf("recomputed balances were logged: outbox %d -> %d entries", logged, got)
	}
}

// Only tables whose rows were applied clear their cache buckets; refused
// rows don't.
func TestApply_InvalidatesCachesOfAppliedTablesOnly(t *testing.T) {
	var keys []string
	prev := syncpkg.InvalidateCache
	syncpkg.InvalidateCache = func(key string) { keys = append(keys, key) }
	t.Cleanup(func() { syncpkg.InvalidateCache = prev })

	peer, node := newNodePair(t)
	since := maxSeq(t, peer)
	setMark(t, peer, "rooms", "rooms-1", "remote", "")
	mustExec(t, peer, `DELETE FROM versions WHERE id = 'versions-2'`)
	mustApply(t, node, outgoing(t, peer, since))

	sort.Strings(keys)
	if want := []string{"analytics", "appointments", "rooms"}; !reflect.DeepEqual(keys, want) {
		t.Errorf("invalidated cache keys = %v, want %v", keys, want)
	}
}

// Apply counts the rows it wrote or removed, renamed and re-created rows
// included; a row it kept, skipped or refused is only a conflict.
func TestApply_CountsTheRowsItWrote(t *testing.T) {
	peer, node := newNodePair(t)
	mustExec(t, node, `INSERT INTO allergies (id, name) VALUES ('allergies-node', 'Shared Allergy')`)
	mustExec(t, node, `INSERT INTO appointments (id, patient_id, room_id, start_time, end_time, status) VALUES ('appointments-node', 'patients-2', 'rooms-1', '2026-01-10T08:00:00Z', '2026-01-10T09:00:00Z', 'Scheduled')`)
	mustExec(t, node, `DELETE FROM rooms WHERE id = 'rooms-2'`)
	since := maxSeq(t, peer)
	setMark(t, peer, "rooms", "rooms-1", "remote", "")
	setMark(t, peer, "rooms", "rooms-2", "peer edit", "")
	mustExec(t, peer, `INSERT INTO allergies (id, name) VALUES ('allergies-peer', 'Shared Allergy')`)
	mustExec(t, peer, `DELETE FROM expenses WHERE id = 'expenses-2'`)
	mustExec(t, peer, `DELETE FROM patients WHERE id = 'patients-2'`)
	orphan := syncpkg.LogEntry{
		Seq: 900, Table: "appointments", RowID: "appointments-9", Op: "insert", CreatedAt: fixtureTime,
		RowJSON: json.RawMessage(`{"id":"appointments-9","patient_id":"patients-missing","room_id":"rooms-1",` +
			`"start_time":"2026-02-01T08:00:00Z","end_time":"2026-02-01T09:00:00Z","status":"Scheduled"}`),
	}

	res, err := syncpkg.Apply(node, append(outgoing(t, peer, since), orphan))
	if err != nil {
		t.Fatal(err)
	}
	assertResolutions(t, res.Conflicts,
		"allergies/allergies-peer unique_renamed",
		"appointments/appointments-9 orphan_skipped",
		"patients/patients-2 delete_refused",
		"rooms/rooms-2 delete_overtaken")
	if res.Written != 4 {
		t.Errorf("written = %d, want 4: the edited room, the re-created room, the renamed allergy and the removed expense", res.Written)
	}
}

// One bad row fails the whole batch: nothing is written and the guard is
// lowered by the rollback.
func TestApply_BadRowRollsBackTheBatch(t *testing.T) {
	peer, node := newNodePair(t)
	since := maxSeq(t, peer)
	mustExec(t, peer, `INSERT INTO rooms (id, name, type) VALUES ('rooms-9', 'Peer Room', 'General')`)
	good := outgoing(t, peer, since)

	cases := []struct {
		name  string
		entry syncpkg.LogEntry
		err   string
	}{
		{"missing key", syncpkg.LogEntry{Seq: 900, Table: "patients", RowID: "patients-9", Op: "insert", RowJSON: json.RawMessage(`{"first_name":"No","last_name":"Key"}`)}, "apply patients/patients-9: row_json missing id"},
		{"no row data", syncpkg.LogEntry{Seq: 901, Table: "patients", RowID: "patients-9", Op: "update"}, "apply patients/patients-9: empty row_json"},
		{"broken json", syncpkg.LogEntry{Seq: 902, Table: "patients", RowID: "patients-9", Op: "update", RowJSON: json.RawMessage(`{"id":`)}, "apply patients/patients-9: decode row_json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			batch := append(append([]syncpkg.LogEntry{}, good...), tc.entry)
			res, err := syncpkg.Apply(node, batch)
			if err == nil || !strings.Contains(err.Error(), tc.err) {
				t.Fatalf("Apply error = %v, want it to contain %q", err, tc.err)
			}
			if res.MaxSeq != 0 || res.Written != 0 || res.Conflicts != nil {
				t.Errorf("failed apply returned %+v", res)
			}
			if rowExists(t, node, "rooms", "rooms-9") {
				t.Error("the valid row of the failed batch was kept")
			}
			if g := applyGuard(t, node); g != 0 {
				t.Errorf("apply guard = %d after a failed apply", g)
			}
		})
	}
}

// A row that holds a unique value a different local row already has is
// renamed instead of failing the batch: the row with the greater id takes a
// suffix from its own id, the conflict is logged with both rows, and the
// renamed row goes back out.
func TestApply_SecondaryUniqueClashRenamesTheGreaterRow(t *testing.T) {
	peer, node := newNodePair(t)
	mustExec(t, node, `INSERT INTO allergies (id, name) VALUES ('allergies-node', 'Shared Allergy')`)
	since := maxSeq(t, peer)
	setMark(t, peer, "rooms", "rooms-1", "remote", "")
	mustExec(t, peer, `INSERT INTO allergies (id, name) VALUES ('allergies-peer', 'Shared Allergy')`)

	// 'allergies-peer' has the greater id: the incoming row is renamed.
	batch := outgoing(t, peer, since)
	applied, conflicts := mustApply(t, node, batch)
	if applied != lastSeq(batch) {
		t.Errorf("applied seq = %d, want %d (a renamed row counts as applied)", applied, lastSeq(batch))
	}
	if len(conflicts) != 1 || conflicts[0].Resolution != "unique_renamed" || conflicts[0].RowID != "allergies-peer" {
		t.Fatalf("conflicts = %+v, want one unique_renamed for allergies-peer", conflicts)
	}
	if jsonField(t, conflicts[0].LocalJSON, "name") != "Shared Allergy" || jsonField(t, conflicts[0].RemoteJSON, "name") != "Shared Allergy" {
		t.Errorf("conflict = %+v, want both rows' bodies", conflicts[0])
	}
	if got, _ := markOf(t, node, "rooms", "rooms-1"); got != "remote" {
		t.Errorf("rooms-1 = %q, want the rest of the batch applied", got)
	}
	if got := nameOf(t, node, "allergies", "allergies-node"); got != "Shared Allergy" {
		t.Errorf("allergies-node = %q, want the row that kept its name", got)
	}
	if got := nameOf(t, node, "allergies", "allergies-peer"); got != "Shared Allergy (aeeee)" {
		t.Errorf("allergies-peer = %q, want the suffixed name", got)
	}
	if n := count(t, node, `SELECT COUNT(*) FROM sync_log WHERE table_name = 'allergies' AND row_id = 'allergies-peer' AND op = 'update'`); n != 1 {
		t.Errorf("renamed rows in the outbox = %d, want the renamed row re-logged", n)
	}

	// The mirror: a local row with the greater id takes the suffix instead
	// and is re-logged where it stands.
	peer, node = newNodePair(t)
	mustExec(t, node, `INSERT INTO allergies (id, name) VALUES ('allergies-zzz', 'Shared Allergy')`)
	since = maxSeq(t, peer)
	mustExec(t, peer, `INSERT INTO allergies (id, name) VALUES ('allergies-aaa', 'Shared Allergy')`)

	_, conflicts = mustApply(t, node, outgoing(t, peer, since))
	if len(conflicts) != 1 || conflicts[0].Resolution != "unique_renamed" || conflicts[0].RowID != "allergies-zzz" {
		t.Fatalf("conflicts = %+v, want one unique_renamed for allergies-zzz", conflicts)
	}
	if got := nameOf(t, node, "allergies", "allergies-aaa"); got != "Shared Allergy" {
		t.Errorf("allergies-aaa = %q, want the incoming row as it arrived", got)
	}
	if got := nameOf(t, node, "allergies", "allergies-zzz"); got != "Shared Allergy (aee)" {
		t.Errorf("allergies-zzz = %q, want the suffixed name", got)
	}
	if n := count(t, node, `SELECT COUNT(*) FROM sync_log WHERE table_name = 'allergies' AND row_id = 'allergies-zzz' AND op = 'update'`); n != 1 {
		t.Errorf("renamed rows in the outbox = %d, want the renamed row re-logged", n)
	}
}

// A remote delete of a row that still has local children is refused: the
// row stays, it and the rows referencing it go back out, and the rest of the
// batch applies.
func TestApply_DeleteOfParentWithLocalChildrenIsRefused(t *testing.T) {
	peer, node := newNodePair(t)
	mustExec(t, node, `INSERT INTO appointments (id, patient_id, room_id, start_time, end_time, status) VALUES ('appointments-node', 'patients-2', 'rooms-1', '2026-01-10T08:00:00Z', '2026-01-10T09:00:00Z', 'Scheduled')`)
	since := maxSeq(t, peer)
	setMark(t, peer, "rooms", "rooms-1", "remote", "")
	mustExec(t, peer, `DELETE FROM patients WHERE id = 'patients-2'`)

	_, conflicts := mustApply(t, node, outgoing(t, peer, since))
	if len(conflicts) != 1 || conflicts[0].Resolution != "delete_refused" || conflicts[0].RowID != "patients-2" {
		t.Fatalf("conflicts = %+v, want one delete_refused for patients-2", conflicts)
	}
	if jsonField(t, conflicts[0].LocalJSON, "id") != "patients-2" || conflicts[0].RemoteJSON != "" {
		t.Errorf("conflict = %+v, want the kept parent's body and no remote row", conflicts[0])
	}
	if !rowExists(t, node, "patients", "patients-2") {
		t.Error("patients-2 was deleted")
	}
	if !rowExists(t, node, "appointments", "appointments-node") {
		t.Error("appointments-node was lost")
	}
	if got, _ := markOf(t, node, "rooms", "rooms-1"); got != "remote" {
		t.Errorf("rooms-1 = %q, want the rest of the batch applied", got)
	}
	if n := count(t, node, `SELECT COUNT(*) FROM sync_log WHERE op = 'update'
		AND ((table_name = 'patients' AND row_id = 'patients-2')
			OR (table_name = 'appointments' AND row_id = 'appointments-node'))`); n != 2 {
		t.Errorf("re-logged rows in the outbox = %d, want the parent and its child", n)
	}
}

// Entries for tables that aren't synced are skipped and don't count toward
// the applied sequence; an empty batch is a no-op.
func TestApply_SkipsUnknownTablesAndEmptyBatches(t *testing.T) {
	peer, node := newNodePair(t)
	since := maxSeq(t, peer)
	setMark(t, peer, "rooms", "rooms-1", "remote", "")
	batch := outgoing(t, peer, since)
	batch = append(batch, syncpkg.LogEntry{Seq: lastSeq(batch) + 100, Table: "tokens", RowID: "token-1", Op: "insert", RowJSON: json.RawMessage(`{"id":"token-1"}`)})

	applied, conflicts := mustApply(t, node, batch)
	if applied != lastSeq(batch)-100 {
		t.Errorf("applied seq = %d, want %d (the unknown table's seq is ignored)", applied, lastSeq(batch)-100)
	}
	if len(conflicts) != 0 {
		t.Errorf("conflicts = %+v", conflicts)
	}
	if got, _ := markOf(t, node, "rooms", "rooms-1"); got != "remote" {
		t.Errorf("rooms-1 = %q, want the remote row", got)
	}
	if n := count(t, node, `SELECT COUNT(*) FROM tokens WHERE id = 'token-1'`); n != 0 {
		t.Errorf("a row for an unsynced table was written")
	}

	res, err := syncpkg.Apply(node, nil)
	if res.MaxSeq != 0 || res.Written != 0 || res.Conflicts != nil || err != nil {
		t.Errorf("Apply(empty) = %+v, %v", res, err)
	}
}

// When the row is gone by the time a batch is built, its entry ships as a
// delete without row data, whatever the logged operation was.
func TestEnrichBatch_MissingRowsBecomeDeletes(t *testing.T) {
	peer := newNode(t)
	loadFixture(t, peer)
	since := maxSeq(t, peer)
	setMark(t, peer, "patients", "patients-2", "edited", "2026-02-01T00:00:00Z")
	batch, err := syncpkg.LoadBatch(peer, since, 10)
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, peer, `DELETE FROM patients WHERE id = 'patients-2'`)
	if err := syncpkg.EnrichBatch(peer, batch); err != nil {
		t.Fatal(err)
	}
	if len(batch) != 1 || batch[0].Op != "delete" || batch[0].RowJSON != nil || batch[0].UpdatedAt != "" {
		t.Fatalf("batch = %+v, want one delete without row data", batch)
	}
}

// Rows ship as JSON of their live columns with updated_at copied alongside.
func TestEnrichBatch_ShipsLiveRows(t *testing.T) {
	peer := newNode(t)
	loadFixture(t, peer)
	since := maxSeq(t, peer)
	setMark(t, peer, "patients", "patients-1", "edited", "2026-02-01T00:00:00Z")
	setMark(t, peer, "roles", "fixture-role", "Edited Role", "")
	batch := outgoing(t, peer, since)
	if len(batch) != 2 {
		t.Fatalf("batch has %d entries, want 2", len(batch))
	}
	byTable := map[string]syncpkg.LogEntry{}
	for _, e := range batch {
		byTable[e.Table] = e
	}
	p := byTable["patients"]
	if p.Op != "update" || p.RowID != "patients-1" || p.UpdatedAt != "2026-02-01T00:00:00Z" || jsonField(t, string(p.RowJSON), "notes") != "edited" {
		t.Errorf("patients entry = %+v", p)
	}
	r := byTable["roles"]
	if r.RowID != "fixture-role" || r.UpdatedAt != "" || jsonField(t, string(r.RowJSON), "label") != "Edited Role" {
		t.Errorf("roles entry = %+v", r)
	}
}
