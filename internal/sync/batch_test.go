package sync_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"testing"

	syncpkg "clinic-api/internal/sync"
)

// A price whose product was written after it (the outbox holds one entry per
// row at its latest write, so the product's entry sits behind the price's)
// ships the product along. The batch still ends at the prefix's last seq,
// and acknowledging that seq keeps the product's own entry pending.
func TestBuildBatch_CarriesPendingParents(t *testing.T) {
	peer := newNode(t)
	loadFixture(t, peer)
	since := maxSeq(t, peer)
	mustExec(t, peer, `INSERT INTO products (id, name, category_id, quantity) VALUES ('products-9', 'Peer Product', 'product_categories-1', 3)`)
	mustExec(t, peer, `INSERT INTO product_prices (id, product_id, price, is_active) VALUES ('product_prices-9', 'products-9', 15, 1)`)
	mustExec(t, peer, `UPDATE products SET name = 'Peer Product v2' WHERE id = 'products-9'`)

	batch, err := syncpkg.BuildBatch(peer, since, 1)
	if err != nil {
		t.Fatal(err)
	}
	if batch.Prefix != 1 || len(batch.Rows) != 2 {
		t.Fatalf("batch = %d rows (%d prefix), want the price plus its product", len(batch.Rows), batch.Prefix)
	}
	dep, row := batch.Rows[0], batch.Rows[1]
	if dep.Table != "products" || dep.RowID != "products-9" || dep.Seq != 0 || dep.Op != "update" {
		t.Errorf("carried parent = %+v, want products/products-9 with seq 0 at its logged op", dep)
	}
	if jsonField(t, string(dep.RowJSON), "name") != "Peer Product v2" {
		t.Errorf("carried parent = %s, want the live row", dep.RowJSON)
	}
	if row.Table != "product_prices" || row.RowID != "product_prices-9" {
		t.Errorf("prefix row = %+v, want the price", row)
	}
	if !batch.Full(1) {
		t.Error("a filled prefix reported no more entries behind it")
	}

	node := newNode(t)
	loadFixture(t, node)
	applied, conflicts := mustApply(t, node, batch.Rows)
	if len(conflicts) != 0 {
		t.Fatalf("conflicts = %+v", conflicts)
	}
	if applied != batch.LastSeq() {
		t.Errorf("applied seq = %d, want the prefix end %d (a carried parent never moves it)", applied, batch.LastSeq())
	}
	if !rowExists(t, node, "products", "products-9") || !rowExists(t, node, "product_prices", "product_prices-9") {
		t.Error("the price or its product is missing on the receiver")
	}

	// Acknowledging the prefix end prunes only the price; the product still
	// travels in its own batch.
	if _, err := syncpkg.PruneOutgoing(peer, batch.LastSeq()); err != nil {
		t.Fatal(err)
	}
	if n := count(t, peer, `SELECT COUNT(*) FROM sync_log WHERE table_name = 'products' AND row_id = 'products-9'`); n != 1 {
		t.Errorf("the carried parent's own outbox entries = %d, want 1 (the ack must not prune it)", n)
	}
}

// An invoice line whose invoice and balance were written after it carries the
// whole chain: dependencies are walked transitively.
func TestBuildBatch_CarriesParentChains(t *testing.T) {
	peer := newNode(t)
	loadFixture(t, peer)
	since := maxSeq(t, peer)
	mustExec(t, peer, `INSERT INTO invoices (id, invoice_number, from_balance_id, to_balance_id, amount, final_amount)
		VALUES ('invoices-9', 9, 'balances-1b', 'balances-1', 60, 60)`)
	mustExec(t, peer, `INSERT INTO invoice_items (id, invoice_id, item_type, item_id, quantity, amount, final_amount)
		VALUES ('invoice_items-9', 'invoices-9', 'product', 'products-1', 3, 60, 60)`)
	mustExec(t, peer, `UPDATE balances SET entity_name = 'Fixture Supplier v2' WHERE id = 'balances-1b'`)
	mustExec(t, peer, `UPDATE invoices SET notes = 'moved behind its line' WHERE id = 'invoices-9'`)

	batch, err := syncpkg.BuildBatch(peer, since, 1)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range batch.Rows {
		got = append(got, e.Table+"/"+e.RowID)
	}
	want := []string{"balances/balances-1b", "invoices/invoices-9", "invoice_items/invoice_items-9"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("batch = %v, want the line plus its invoice and balance parents-first %v", got, want)
	}
	for _, e := range batch.Rows[:2] {
		if e.Seq != 0 {
			t.Errorf("carried %s/%s has seq %d, want 0", e.Table, e.RowID, e.Seq)
		}
	}
	if batch.LastSeq() != batch.Rows[2].Seq {
		t.Errorf("batch ends at seq %d, want the prefix row's %d", batch.LastSeq(), batch.Rows[2].Seq)
	}

	node := newNode(t)
	loadFixture(t, node)
	mustApply(t, node, batch.Rows)
	for _, id := range []string{"balances-1b", "invoices-9", "invoice_items-9"} {
		table := map[string]string{"balances-1b": "balances", "invoices-9": "invoices", "invoice_items-9": "invoice_items"}[id]
		if !rowExists(t, node, table, id) {
			t.Errorf("%s/%s is missing on the receiver", table, id)
		}
	}
}

// Two prices of one later-updated product carry the product once, not once
// per child.
func TestBuildBatch_CarriesARepeatedParentOnce(t *testing.T) {
	peer := newNode(t)
	loadFixture(t, peer)
	since := maxSeq(t, peer)
	mustExec(t, peer, `INSERT INTO products (id, name, category_id, quantity) VALUES ('products-9', 'Peer Product', 'product_categories-1', 3)`)
	mustExec(t, peer, `INSERT INTO product_prices (id, product_id, price, is_active) VALUES ('product_prices-9a', 'products-9', 15, 1)`)
	mustExec(t, peer, `INSERT INTO product_prices (id, product_id, price, is_active) VALUES ('product_prices-9b', 'products-9', 20, 1)`)
	mustExec(t, peer, `UPDATE products SET name = 'Peer Product v2' WHERE id = 'products-9'`)

	batch, err := syncpkg.BuildBatch(peer, since, 2)
	if err != nil {
		t.Fatal(err)
	}
	if batch.Prefix != 2 || len(batch.Rows) != 3 {
		t.Fatalf("batch = %d rows (%d prefix), want the two prices plus one carried product", len(batch.Rows), batch.Prefix)
	}
	if batch.Rows[0].Table != "products" || batch.Rows[0].RowID != "products-9" || batch.Rows[0].Seq != 0 {
		t.Errorf("carried parent = %+v, want products/products-9 once with seq 0", batch.Rows[0])
	}
	for _, e := range batch.Rows[1:] {
		if e.Table != "product_prices" || e.Seq == 0 {
			t.Errorf("prefix row = %+v, want the price entries", e)
		}
	}
}

// Replaying a whole outbox into a fresh node, one small batch at a time,
// ends with the same rows in every synced table, whatever the batch size.
// No row travels twice in one batch, and the small limits really exercise
// the carry.
func TestBuildBatch_ReplaysOutboxAtAnyBatchSize(t *testing.T) {
	for _, limit := range []int{1, 2, 7, 500} {
		t.Run(fmt.Sprintf("limit %d", limit), func(t *testing.T) {
			peer := newNode(t)
			loadFixture(t, peer)
			// Rewrite every base row children first, so each parent's entry
			// lands behind its children's: small batches must carry parents
			// to converge.
			for i := len(syncpkg.SyncedTables) - 1; i >= 0; i-- {
				ti := syncpkg.SyncedTables[i]
				mark, _ := markOf(t, peer, ti.Name, baseRowID(ti.Name))
				mustExec(t, peer, fmt.Sprintf(`UPDATE %q SET %q = ? WHERE %q = ?`,
					ti.Name, markColumn[ti.Name], ti.PK()), mark, baseRowID(ti.Name))
			}

			node := newNode(t)
			var cursor int64
			carried := 0
			for {
				batch, err := syncpkg.BuildBatch(peer, cursor, limit)
				if err != nil {
					t.Fatal(err)
				}
				if len(batch.Rows) == 0 {
					break
				}
				if batch.LastSeq() <= cursor {
					t.Fatalf("batch ended at seq %d, cursor already %d", batch.LastSeq(), cursor)
				}
				seen := make(map[string]struct{}, len(batch.Rows))
				for _, e := range batch.Rows {
					key := e.Table + "/" + e.RowID
					if _, dup := seen[key]; dup {
						t.Fatalf("%s travels twice in one batch", key)
					}
					seen[key] = struct{}{}
				}
				carried += len(batch.Rows) - batch.Prefix
				if _, err := syncpkg.Apply(node, batch.Rows); err != nil {
					t.Fatalf("apply batch of %d rows: %v", len(batch.Rows), err)
				}
				cursor = batch.LastSeq()
			}
			if cursor != maxSeq(t, peer) {
				t.Errorf("replay stopped at seq %d, want %d", cursor, maxSeq(t, peer))
			}
			if limit < 500 && carried == 0 {
				t.Errorf("no batch carried a pending parent at limit %d", limit)
			}
			assertSameSyncedTables(t, peer, node)
		})
	}
}

// assertSameSyncedTables compares, for every synced table, the rows the
// fixture wrote on the two databases. Rows the migrations seeded stay out:
// their ids and timestamps differ per database, and the versions seed and
// the countries repair are logged (their migrations run after the sync
// triggers) and sync too, so the receiver ends up with its own row next to
// the synced one.
func assertSameSyncedTables(t *testing.T, want, got *sql.DB) {
	t.Helper()
	fixture := map[string]map[string]struct{}{}
	for _, r := range fixtureRows {
		if fixture[r.table] == nil {
			fixture[r.table] = map[string]struct{}{}
		}
		fixture[r.table][r.id] = struct{}{}
	}
	for _, ti := range syncpkg.SyncedTables {
		ids := fixture[ti.Name]
		if len(ids) == 0 {
			continue
		}
		a := tableSnapshot(t, want, ti, ids)
		b := tableSnapshot(t, got, ti, ids)
		for id, line := range a {
			gotLine, ok := b[id]
			switch {
			case !ok:
				t.Errorf("%s/%s is missing on the receiver", ti.Name, id)
			case line != gotLine:
				t.Errorf("%s/%s = %s on the peer and %s on the receiver", ti.Name, id, line, gotLine)
			}
		}
		for id := range b {
			if _, ok := a[id]; !ok {
				t.Errorf("%s/%s is on the receiver but not the peer", ti.Name, id)
			}
		}
	}
}

// tableSnapshot returns the selected rows as canonical JSON, keyed by
// primary key.
func tableSnapshot(t *testing.T, db *sql.DB, ti syncpkg.TableInfo, ids map[string]struct{}) map[string]string {
	t.Helper()
	rows, err := db.Query(fmt.Sprintf(`SELECT * FROM %q ORDER BY %q`, ti.Name, ti.PK()))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]string, len(ids))
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		row := make(map[string]any, len(cols))
		var pk string
		for i, c := range cols {
			v := vals[i]
			if b, ok := v.([]byte); ok {
				v = string(b)
			}
			row[c] = v
			if c == ti.PK() {
				pk, _ = v.(string)
			}
		}
		if _, wanted := ids[pk]; !wanted {
			continue
		}
		buf, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		out[pk] = string(buf)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// A rescheduled appointment listed before its original (the original was
// edited after the reschedule, which moves its entry behind the new row)
// applies inside one batch.
func TestApply_RescheduledAppointmentBeforeOriginal(t *testing.T) {
	peer, node := newNodePair(t)
	since := maxSeq(t, peer)
	mustExec(t, peer, `INSERT INTO appointments (id, patient_id, room_id, start_time, end_time, status)
		VALUES ('appointments-o', 'patients-1', 'rooms-1', '2026-02-01T08:00:00Z', '2026-02-01T09:00:00Z', 'Scheduled')`)
	mustExec(t, peer, `INSERT INTO appointments (id, patient_id, room_id, start_time, end_time, status, rescheduled_from)
		VALUES ('appointments-n', 'patients-1', 'rooms-1', '2026-02-02T08:00:00Z', '2026-02-02T09:00:00Z', 'Scheduled', 'appointments-o')`)
	mustExec(t, peer, `UPDATE appointments SET status = 'Rescheduled' WHERE id = 'appointments-o'`)

	batch := outgoing(t, peer, since)
	if len(batch) != 2 || batch[0].RowID != "appointments-n" || batch[1].RowID != "appointments-o" {
		t.Fatalf("batch = %+v, want the rescheduled row before its original", batch)
	}
	mustApply(t, node, batch)
	for _, id := range []string{"appointments-n", "appointments-o"} {
		if !rowExists(t, node, "appointments", id) {
			t.Errorf("appointments/%s is missing on the receiver", id)
		}
	}
}

// A violation that predates the batch, in a table the batch never touches,
// does not fail the apply: the check probes the batch's own rows, and the
// deferred commit check re-checks only constraints the transaction touched.
func TestApply_IgnoresAViolationOutsideTheBatch(t *testing.T) {
	peer, node := newNodePair(t)
	mustExec(t, node, `PRAGMA foreign_keys = OFF`)
	mustExec(t, node, `INSERT INTO appointments (id, patient_id, room_id, start_time, end_time, status)
		VALUES ('appointments-orphan', 'patients-missing', 'rooms-1', '2026-02-01T08:00:00Z', '2026-02-01T09:00:00Z', 'Scheduled')`)
	mustExec(t, node, `PRAGMA foreign_keys = ON`)

	since := maxSeq(t, peer)
	setMark(t, peer, "rooms", "rooms-1", "remote", "")
	_, conflicts := mustApply(t, node, outgoing(t, peer, since))
	if len(conflicts) != 0 {
		t.Fatalf("conflicts = %+v", conflicts)
	}
	if got, _ := markOf(t, node, "rooms", "rooms-1"); got != "remote" {
		t.Errorf("rooms-1 = %q, want the batch applied", got)
	}
}

// An upsert whose parent is missing everywhere is skipped with its data
// logged, and the rest of the batch applies; the row returns with its
// parent.
func TestApply_OrphanIsSkippedUntilItsParentArrives(t *testing.T) {
	peer, node := newNodePair(t)
	since := maxSeq(t, peer)
	setMark(t, peer, "rooms", "rooms-1", "remote", "")
	orphan := syncpkg.LogEntry{
		Seq: 900, Table: "appointments", RowID: "appointments-9", Op: "insert", CreatedAt: fixtureTime,
		RowJSON: json.RawMessage(`{"id":"appointments-9","patient_id":"patients-missing","room_id":"rooms-1",` +
			`"start_time":"2026-02-01T08:00:00Z","end_time":"2026-02-01T09:00:00Z","status":"Scheduled"}`),
	}
	batch := append(outgoing(t, peer, since), orphan)

	applied, conflicts := mustApply(t, node, batch)
	if applied != lastSeq(batch) {
		t.Errorf("applied seq = %d, want %d (a skipped row counts as handled)", applied, lastSeq(batch))
	}
	if len(conflicts) != 1 || conflicts[0].Resolution != "orphan_skipped" || conflicts[0].RowID != "appointments-9" {
		t.Fatalf("conflicts = %+v, want one orphan_skipped for appointments-9", conflicts)
	}
	if jsonField(t, conflicts[0].RemoteJSON, "patient_id") != "patients-missing" {
		t.Errorf("conflict = %+v, want the skipped row's data", conflicts[0])
	}
	if rowExists(t, node, "appointments", "appointments-9") {
		t.Error("the orphan was written")
	}
	if got, _ := markOf(t, node, "rooms", "rooms-1"); got != "remote" {
		t.Errorf("rooms-1 = %q, want the rest of the batch applied", got)
	}

	parent := syncpkg.LogEntry{
		Seq: 901, Table: "patients", RowID: "patients-missing", Op: "insert", CreatedAt: fixtureTime,
		RowJSON: json.RawMessage(`{"id":"patients-missing","first_name":"Found","last_name":"Parent",` +
			`"date_of_birth":"1990-01-01","contact":""}`),
	}
	_, conflicts = mustApply(t, node, []syncpkg.LogEntry{parent, orphan})
	if len(conflicts) != 0 {
		t.Fatalf("conflicts = %+v, want the row to arrive with its parent", conflicts)
	}
	if !rowExists(t, node, "patients", "patients-missing") || !rowExists(t, node, "appointments", "appointments-9") {
		t.Error("the skipped row did not return with its parent")
	}
}

// A parent the batch brings but skips (its own parent is missing) is skipped
// before the row listed ahead of it that references it, so that row is
// skipped too and the batch applies.
func TestApply_SkippedParentSkipsTheRowReferencingIt(t *testing.T) {
	node := newNode(t)
	loadFixture(t, node)
	entries := []syncpkg.LogEntry{
		{Seq: 900, Table: "appointments", RowID: "apt-rescheduled", Op: "insert", CreatedAt: fixtureTime,
			RowJSON: json.RawMessage(`{"id":"apt-rescheduled","patient_id":"patients-1","room_id":"rooms-1",` +
				`"start_time":"2026-02-01T08:00:00Z","end_time":"2026-02-01T09:00:00Z","status":"Scheduled","rescheduled_from":"apt-original"}`)},
		{Seq: 901, Table: "appointments", RowID: "apt-original", Op: "insert", CreatedAt: fixtureTime,
			RowJSON: json.RawMessage(`{"id":"apt-original","patient_id":"patients-missing","room_id":"rooms-1",` +
				`"start_time":"2026-02-01T08:00:00Z","end_time":"2026-02-01T09:00:00Z","status":"Scheduled"}`)},
	}

	applied, conflicts := mustApply(t, node, entries)
	if applied != 901 {
		t.Errorf("applied seq = %d, want 901 (skipped rows count as handled)", applied)
	}
	assertResolutions(t, conflicts,
		"appointments/apt-original orphan_skipped",
		"appointments/apt-rescheduled orphan_skipped")
	for _, id := range []string{"apt-original", "apt-rescheduled"} {
		if rowExists(t, node, "appointments", id) {
			t.Errorf("appointments/%s was written", id)
		}
	}
}

// While apart, the receiver deletes a room; the other node books an
// appointment in it, reschedules it, and edits the original again, so the
// rescheduled row is listed first. The original is skipped as an orphan and
// the reschedule with it, so the batch applies; the receiver's delete of the
// room then reaches the other node, which refuses it, and the room comes
// back with both appointments.
func TestApply_OrphanedOriginalSkipsItsReschedule(t *testing.T) {
	peer, node := newNodePair(t)
	execAll(t, []string{`INSERT INTO rooms (id, name, type) VALUES ('rooms-9', 'Systest Room', 'General')`}, peer, node)
	down, up := newLink(t, peer, node), newLink(t, node, peer)
	mustExec(t, node, `DELETE FROM rooms WHERE id = 'rooms-9'`)
	mustExec(t, peer, `INSERT INTO appointments (id, patient_id, room_id, start_time, end_time, status)
		VALUES ('appointments-o', 'patients-1', 'rooms-9', '2026-02-01T08:00:00Z', '2026-02-01T09:00:00Z', 'Scheduled')`)
	mustExec(t, peer, `INSERT INTO appointments (id, patient_id, room_id, start_time, end_time, status, rescheduled_from)
		VALUES ('appointments-n', 'patients-1', 'rooms-1', '2026-02-02T08:00:00Z', '2026-02-02T09:00:00Z', 'Scheduled', 'appointments-o')`)
	mustExec(t, peer, `UPDATE appointments SET status = 'Rescheduled' WHERE id = 'appointments-o'`)

	assertResolutions(t, down.ship(t),
		"appointments/appointments-n orphan_skipped",
		"appointments/appointments-o orphan_skipped")

	assertResolutions(t, settle(t, up, down), "rooms/rooms-9 delete_refused")
	assertSameRows(t, peer, node, "rooms", "rooms-9")
	assertSameRows(t, peer, node, "appointments", "appointments-o", "appointments-n")
}

// A reference holding the empty string names a parent like any other value:
// SQLite exempts only NULL. A row whose referenced parent is missing is
// skipped, so the rest of the batch applies.
func TestApply_EmptyReferenceNamesAMissingParent(t *testing.T) {
	peer, node := newNodePair(t)
	since := maxSeq(t, peer)
	setMark(t, peer, "rooms", "rooms-1", "remote", "")
	unsigned := syncpkg.LogEntry{
		Seq: 900, Table: "prescriptions", RowID: "prescriptions-9", Op: "insert", CreatedAt: fixtureTime,
		RowJSON: json.RawMessage(`{"id":"prescriptions-9","patient_id":"patients-1","prescribed_by_id":"",` +
			`"start_date":"2026-02-01","end_date":""}`),
	}
	_, conflicts := mustApply(t, node, append(outgoing(t, peer, since), unsigned))
	assertResolutions(t, conflicts, "prescriptions/prescriptions-9 orphan_skipped")
	if rowExists(t, node, "prescriptions", "prescriptions-9") {
		t.Error("the row with the empty reference was written")
	}
	if got, _ := markOf(t, node, "rooms", "rooms-1"); got != "remote" {
		t.Errorf("rooms-1 = %q, want the rest of the batch applied", got)
	}
}

// The parent references the batch builder walks are derived from the schema;
// this pins them so a schema change is noticed.
func TestParentRefs_MatchTheSchema(t *testing.T) {
	got, _, err := syncpkg.ReadForeignKeys(newNode(t))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]syncpkg.FKRef{
		"appointments":                 {{"patient_id", "patients"}, {"room_id", "rooms"}, {"rescheduled_from", "appointments"}},
		"appointment_procedures":       {{"patient_id", "patients"}, {"procedure_id", "procedures"}, {"appointment_id", "appointments"}, {"assigned_to_id", "employees"}},
		"patient_allergies":            {{"patient_id", "patients"}, {"allergy_id", "allergies"}},
		"patient_medicines":            {{"patient_id", "patients"}, {"medicine_id", "medicines"}},
		"prescriptions":                {{"patient_id", "patients"}, {"prescribed_by_id", "employees"}},
		"prescription_medicines":       {{"medicine_id", "medicines"}, {"prescription_id", "prescriptions"}},
		"procedure_prices":             {{"procedure_id", "procedures"}},
		"procedure_allergy_conflicts":  {{"procedure_id", "procedures"}, {"allergy_id", "allergies"}},
		"product_prices":               {{"product_id", "products"}},
		"product_allergy_conflicts":    {{"product_id", "products"}, {"allergy_id", "allergies"}},
		"employee_schedules":           {{"employee_id", "employees"}},
		"employee_schedule_changes":    {{"employee_id", "employees"}},
		"employee_salaries":            {{"employee_id", "employees"}},
		"employee_salary_preparations": {{"employee_id", "employees"}},
		"balance_transactions":         {{"from_balance_id", "balances"}, {"to_balance_id", "balances"}},
		"invoices":                     {{"from_balance_id", "balances"}, {"to_balance_id", "balances"}},
		"invoice_items":                {{"invoice_id", "invoices"}},
	}
	sortRefs := func(m map[string][]syncpkg.FKRef) map[string][]syncpkg.FKRef {
		out := make(map[string][]syncpkg.FKRef, len(m))
		for table, refs := range m {
			sorted := append([]syncpkg.FKRef(nil), refs...)
			sort.Slice(sorted, func(i, j int) bool { return sorted[i].Column < sorted[j].Column })
			out[table] = sorted
		}
		return out
	}
	if !reflect.DeepEqual(sortRefs(got), sortRefs(want)) {
		t.Errorf("parent references =\n%v\nwant\n%v", sortRefs(got), sortRefs(want))
	}

	// Apply decides a row's parents before the row by walking SyncedTables in
	// order, so every other table a table references must come before it.
	rank := make(map[string]int, len(syncpkg.SyncedTables))
	for i, ti := range syncpkg.SyncedTables {
		rank[ti.Name] = i
	}
	for table, refs := range got {
		for _, ref := range refs {
			if ref.Parent != table && rank[ref.Parent] > rank[table] {
				t.Errorf("%s references %s, which comes after it in SyncedTables", table, ref.Parent)
			}
		}
	}
}
