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
			applied, conflicts, err := syncpkg.Apply(node, batch)
			if err == nil || !strings.Contains(err.Error(), tc.err) {
				t.Fatalf("Apply error = %v, want it to contain %q", err, tc.err)
			}
			if applied != 0 || conflicts != nil {
				t.Errorf("failed apply returned seq %d and %d conflicts", applied, len(conflicts))
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

// A row that collides with a different local row on a secondary unique key
// (here the allergy name) fails the whole batch, on both builds.
func TestApply_SecondaryUniqueKeyCollisionFailsTheBatch(t *testing.T) {
	peer, node := newNodePair(t)
	mustExec(t, node, `INSERT INTO allergies (id, name) VALUES ('allergies-node', 'Shared Allergy')`)
	since := maxSeq(t, peer)
	setMark(t, peer, "rooms", "rooms-1", "remote", "")
	mustExec(t, peer, `INSERT INTO allergies (id, name) VALUES ('allergies-peer', 'Shared Allergy')`)

	_, _, err := syncpkg.Apply(node, outgoing(t, peer, since))
	if err == nil || !strings.Contains(err.Error(), "apply allergies/allergies-peer") || !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("Apply error = %v, want a unique constraint failure on allergies-peer", err)
	}
	if got, _ := markOf(t, node, "rooms", "rooms-1"); got != "Fixture Room" {
		t.Errorf("rooms-1 = %q, want the batch rolled back", got)
	}
}

// A remote delete of a row that still has local children fails the whole
// batch with a foreign key error, on both builds.
func TestApply_DeleteOfParentWithLocalChildrenFailsTheBatch(t *testing.T) {
	peer, node := newNodePair(t)
	mustExec(t, node, `INSERT INTO appointments (id, patient_id, room_id, start_time, end_time, status) VALUES ('appointments-node', 'patients-2', 'rooms-1', '2026-01-10T08:00:00Z', '2026-01-10T09:00:00Z', 'Scheduled')`)
	since := maxSeq(t, peer)
	setMark(t, peer, "rooms", "rooms-1", "remote", "")
	mustExec(t, peer, `DELETE FROM patients WHERE id = 'patients-2'`)

	_, _, err := syncpkg.Apply(node, outgoing(t, peer, since))
	if err == nil || !strings.Contains(err.Error(), "delete patients/patients-2") || !strings.Contains(err.Error(), "FOREIGN KEY") {
		t.Fatalf("Apply error = %v, want a foreign key failure deleting patients-2", err)
	}
	if !rowExists(t, node, "patients", "patients-2") {
		t.Error("patients-2 was deleted")
	}
	if got, _ := markOf(t, node, "rooms", "rooms-1"); got != "Fixture Room" {
		t.Errorf("rooms-1 = %q, want the batch rolled back", got)
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

	applied, conflicts, err := syncpkg.Apply(node, nil)
	if applied != 0 || conflicts != nil || err != nil {
		t.Errorf("Apply(empty) = %d, %v, %v", applied, conflicts, err)
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
