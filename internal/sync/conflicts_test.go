package sync_test

import (
	"database/sql"
	"fmt"
	"reflect"
	"sort"
	"testing"

	syncpkg "clinic-api/internal/sync"
)

// nameOf reads one row's name column, the unique key the clash tests work on.
func nameOf(t *testing.T, db *sql.DB, table, id string) string {
	t.Helper()
	var name string
	if err := db.QueryRow(fmt.Sprintf(`SELECT name FROM %q WHERE id = ?`, table), id).Scan(&name); err != nil {
		t.Fatalf("read %s/%s name: %v", table, id, err)
	}
	return name
}

// refOf reads one reference column of a row.
func refOf(t *testing.T, db *sql.DB, table, id, column string) string {
	t.Helper()
	var ref sql.NullString
	if err := db.QueryRow(fmt.Sprintf(`SELECT %q FROM %q WHERE id = ?`, column, table), id).Scan(&ref); err != nil {
		t.Fatalf("read %s/%s %s: %v", table, id, column, err)
	}
	return ref.String
}

// link is one direction of the exchange between two test nodes, with the
// sender's outbox cursor as a push or a pull keeps it.
type link struct {
	from, to *sql.DB
	cursor   int64
}

// newLink starts a direction at the sender's current outbox end, so only
// what the sender writes from now on travels.
func newLink(t *testing.T, from, to *sql.DB) *link {
	t.Helper()
	return &link{from: from, to: to, cursor: maxSeq(t, from)}
}

// ship sends what the sender logged after the cursor, built the way a push
// or a pull builds its batch, applies it on the receiver and acknowledges it:
// the cursor moves to the batch's last entry and the sender's outbox is
// pruned through it. Returns the conflicts the receiver recorded.
func (l *link) ship(t *testing.T) []syncpkg.ConflictEntry {
	t.Helper()
	batch := outgoing(t, l.from, l.cursor)
	if len(batch) == 0 {
		return nil
	}
	_, conflicts := mustApply(t, l.to, batch)
	l.cursor = lastSeq(batch)
	if _, err := syncpkg.PruneOutgoing(l.from, l.cursor); err != nil {
		t.Fatal(err)
	}
	return conflicts
}

// settle ships along the links in turn until no sender has anything left,
// and returns every conflict recorded on the way. Sync must come to rest: a
// pair that still sends after a few rounds fails the test.
func settle(t *testing.T, links ...*link) []syncpkg.ConflictEntry {
	t.Helper()
	var all []syncpkg.ConflictEntry
	for round := 0; round < 5; round++ {
		moved := false
		for _, l := range links {
			before := l.cursor
			all = append(all, l.ship(t)...)
			moved = moved || l.cursor != before
		}
		if !moved {
			return all
		}
	}
	t.Fatalf("the nodes still send after 5 rounds; conflicts so far: %+v", all)
	return nil
}

// resolutions lists conflicts as "table/id resolution", sorted.
func resolutions(conflicts []syncpkg.ConflictEntry) []string {
	out := make([]string, 0, len(conflicts))
	for _, c := range conflicts {
		out = append(out, c.Table+"/"+c.RowID+" "+c.Resolution)
	}
	sort.Strings(out)
	return out
}

func assertResolutions(t *testing.T, conflicts []syncpkg.ConflictEntry, want ...string) {
	t.Helper()
	sort.Strings(want)
	if got := resolutions(conflicts); !reflect.DeepEqual(got, want) {
		t.Fatalf("conflicts = %v, want %v", got, want)
	}
}

// assertSameRows checks that two nodes hold the named rows alike: each row on
// both, with the same columns.
func assertSameRows(t *testing.T, a, b *sql.DB, table string, ids ...string) {
	t.Helper()
	ti, _ := syncpkg.IsSyncedTable(table)
	want := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		want[id] = struct{}{}
	}
	rowsA, rowsB := tableSnapshot(t, a, ti, want), tableSnapshot(t, b, ti, want)
	for _, id := range ids {
		switch {
		case rowsA[id] == "" || rowsB[id] == "":
			t.Errorf("%s/%s is missing on a node (%q, %q)", table, id, rowsA[id], rowsB[id])
		case rowsA[id] != rowsB[id]:
			t.Errorf("%s/%s = %s on one node and %s on the other", table, id, rowsA[id], rowsB[id])
		}
	}
}

func execAll(t *testing.T, queries []string, dbs ...*sql.DB) {
	t.Helper()
	for _, db := range dbs {
		for _, q := range queries {
			mustExec(t, db, q)
		}
	}
}

// A row deleted and re-created under the same unique value, both in one
// batch, applies with no rename: the delete runs ahead of the insert that
// reuses its value.
func TestApply_DeleteAndRecreateOfOneUniqueValue(t *testing.T) {
	peer, node := newNodePair(t)
	for _, db := range []*sql.DB{peer, node} {
		mustExec(t, db, `INSERT INTO allergies (id, name) VALUES ('allergies-old', 'Systest Latex')`)
	}
	since := maxSeq(t, peer)
	mustExec(t, peer, `DELETE FROM allergies WHERE id = 'allergies-old'`)
	mustExec(t, peer, `INSERT INTO allergies (id, name) VALUES ('allergies-new', 'Systest Latex')`)
	batch := outgoing(t, peer, since)
	if len(batch) != 2 || batch[0].Op != "delete" || batch[1].Op != "insert" {
		t.Fatalf("batch = %+v, want the delete before the re-created row", batch)
	}

	applied, conflicts := mustApply(t, node, batch)
	if applied != lastSeq(batch) {
		t.Errorf("applied seq = %d, want %d", applied, lastSeq(batch))
	}
	if len(conflicts) != 0 {
		t.Fatalf("conflicts = %+v, want none", conflicts)
	}
	if rowExists(t, node, "allergies", "allergies-old") {
		t.Error("allergies-old survived its own batch's delete")
	}
	if got := nameOf(t, node, "allergies", "allergies-new"); got != "Systest Latex" {
		t.Errorf("allergies-new = %q, want the re-created name", got)
	}
}

// An allergy deleted for its name and re-created, with its patient link
// deleted and made again (link ids derive from the patient and the allergy,
// so the new link is a new row): the batch's link delete goes, so the early
// delete of the old allergy runs and its cascade takes only that link.
func TestApply_DeleteAndRecreateWithItsLinks(t *testing.T) {
	peer, node := newNodePair(t)
	execAll(t, []string{
		`INSERT INTO allergies (id, name) VALUES ('allergies-old', 'Systest Latex')`,
		`INSERT INTO patient_allergies (id, patient_id, allergy_id) VALUES ('pl-old', 'patients-1', 'allergies-old')`,
	}, peer, node)
	since := maxSeq(t, peer)
	mustExec(t, peer, `DELETE FROM allergies WHERE id = 'allergies-old'`)
	mustExec(t, peer, `INSERT INTO allergies (id, name) VALUES ('allergies-new', 'Systest Latex')`)
	mustExec(t, peer, `INSERT INTO patient_allergies (id, patient_id, allergy_id) VALUES ('pl-new', 'patients-1', 'allergies-new')`)

	_, conflicts := mustApply(t, node, outgoing(t, peer, since))
	if len(conflicts) != 0 {
		t.Fatalf("conflicts = %+v, want none", conflicts)
	}
	if rowExists(t, node, "allergies", "allergies-old") || rowExists(t, node, "patient_allergies", "pl-old") {
		t.Error("the old allergy or its link survived the batch's deletes")
	}
	if got := nameOf(t, node, "allergies", "allergies-new"); got != "Systest Latex" {
		t.Errorf("allergies-new = %q, want the re-created name", got)
	}
	if got := refOf(t, node, "patient_allergies", "pl-new", "allergy_id"); got != "allergies-new" {
		t.Errorf("the new link points at %q, want the re-created allergy", got)
	}
}

// A child the batch moves to the re-created row under its own id still
// references the old row when the early delete runs, so that delete is
// refused and the rename resolves the clash. Nothing is lost: the old row
// stays with a suffix and goes back to the other node.
func TestApply_ChildMovedUnderItsOwnIdKeepsTheEarlyDeleteRefused(t *testing.T) {
	peer, node := newNodePair(t)
	execAll(t, []string{
		`INSERT INTO allergies (id, name) VALUES ('allergies-old', 'Systest Latex')`,
		`INSERT INTO patient_allergies (id, patient_id, allergy_id) VALUES ('pl-1', 'patients-1', 'allergies-old')`,
	}, peer, node)
	since := maxSeq(t, peer)
	mustExec(t, peer, `DELETE FROM allergies WHERE id = 'allergies-old'`)
	mustExec(t, peer, `INSERT INTO allergies (id, name) VALUES ('allergies-new', 'Systest Latex')`)
	mustExec(t, peer, `INSERT INTO patient_allergies (id, patient_id, allergy_id) VALUES ('pl-1', 'patients-1', 'allergies-new')`)

	_, conflicts := mustApply(t, node, outgoing(t, peer, since))
	assertResolutions(t, conflicts, "allergies/allergies-old delete_refused", "allergies/allergies-old unique_renamed")
	if got := nameOf(t, node, "allergies", "allergies-new"); got != "Systest Latex" {
		t.Errorf("allergies-new = %q, want the re-created name", got)
	}
	if got := nameOf(t, node, "allergies", "allergies-old"); got != "Systest Latex (aeed)" {
		t.Errorf("allergies-old = %q, want the kept row with its suffix", got)
	}
	if got := refOf(t, node, "patient_allergies", "pl-1", "allergy_id"); got != "allergies-new" {
		t.Errorf("the moved link points at %q, want the re-created allergy", got)
	}
	if n := count(t, node, `SELECT COUNT(*) FROM sync_log WHERE table_name = 'allergies' AND row_id = 'allergies-old' AND op = 'update'`); n != 1 {
		t.Errorf("kept rows in the outbox = %d, want the old allergy re-logged", n)
	}
}

// Two nodes that create the same unique value offline end up with the same
// rows, whichever node's row arrives first: the greater id takes a suffix
// from its own id on both nodes, lengthened past a value another row already
// holds, and the renamed rows' way back to the other node records no new
// conflict, so the exchange comes to rest.
func TestApply_UniqueClashConvergesBothWays(t *testing.T) {
	cases := []struct {
		name    string
		stored  []string // on both nodes before the clash
		greater string   // the id of the row that takes the suffix
		want    map[string]string
	}{
		{
			name:    "a free suffix",
			greater: "users-b",
			want:    map[string]string{"users-a": "systest-clash", "users-b": "systest-clash-eb"},
		},
		{
			name: "the short suffix already taken",
			stored: []string{`INSERT INTO users (id, username, display_name, role)
				VALUES ('users-x', 'systest-clash-abc123', 'Systest X', 'staff')`},
			greater: "users-b-abc123",
			want: map[string]string{
				"users-a":        "systest-clash",
				"users-b-abc123": "systest-clash-ebabc123",
				"users-x":        "systest-clash-abc123",
			},
		},
	}
	for _, tc := range cases {
		for _, peerFirst := range []bool{true, false} {
			order := "the peer's row arrives first"
			if !peerFirst {
				order = "the node's row arrives first"
			}
			t.Run(tc.name+", "+order, func(t *testing.T) {
				peer, node := newNodePair(t)
				execAll(t, tc.stored, peer, node)
				toNode, toPeer := newLink(t, peer, node), newLink(t, node, peer)
				mustExec(t, peer, `INSERT INTO users (id, username, display_name, role) VALUES (?, 'systest-clash', 'Systest B', 'staff')`, tc.greater)
				mustExec(t, node, `INSERT INTO users (id, username, display_name, role) VALUES ('users-a', 'systest-clash', 'Systest A', 'staff')`)
				first, second := toNode, toPeer
				if !peerFirst {
					first, second = toPeer, toNode
				}

				if c := first.ship(t); len(c) != 1 || c[0].Resolution != "unique_renamed" || c[0].RowID != tc.greater {
					t.Fatalf("first arrival: conflicts = %+v, want one unique_renamed for %s", c, tc.greater)
				}
				second.ship(t)
				if c := settle(t, first, second); len(c) != 0 {
					t.Fatalf("the renamed rows' way back: conflicts = %+v, want none", c)
				}

				for _, db := range []*sql.DB{peer, node} {
					for id, name := range tc.want {
						var got string
						if err := db.QueryRow(`SELECT username FROM users WHERE id = ?`, id).Scan(&got); err != nil || got != name {
							t.Errorf("users/%s = %q (%v), want %q", id, got, err, name)
						}
					}
				}
			})
		}
	}
}

// A rename finds no free value when every suffix the greater row's id offers
// is already held: the incoming row is parked instead, so the batch applies.
func TestApply_RenameWithNoFreeSuffixParksTheRow(t *testing.T) {
	peer, node := newNodePair(t)
	execAll(t, []string{`INSERT INTO users (id, username, display_name, role) VALUES ('users-x', 'systest-clash-eb', 'Systest X', 'staff')`}, peer, node)
	mustExec(t, node, `INSERT INTO users (id, username, display_name, role) VALUES ('users-b', 'systest-clash', 'Systest B', 'staff')`)
	since := maxSeq(t, peer)
	setMark(t, peer, "rooms", "rooms-1", "remote", "")
	mustExec(t, peer, `INSERT INTO users (id, username, display_name, role) VALUES ('users-a', 'systest-clash', 'Systest A', 'staff')`)

	_, conflicts := mustApply(t, node, outgoing(t, peer, since))
	assertResolutions(t, conflicts, "users/users-a unique_parked")
	if rowExists(t, node, "users", "users-a") {
		t.Error("the parked row was written")
	}
	if got, _ := markOf(t, node, "rooms", "rooms-1"); got != "remote" {
		t.Errorf("rooms-1 = %q, want the rest of the batch applied", got)
	}
}

// A clash on a key that can't sensibly be renamed parks the incoming row and
// logs the conflict; the row it clashed with keeps its value.
func TestApply_UnrenamableClashParksTheRow(t *testing.T) {
	peer, node := newNodePair(t)
	mustExec(t, node, `INSERT INTO currencies (id, code, name, symbol, exchange_rate) VALUES ('currencies-node', 'TSX', 'Node Currency', 'X', 2)`)
	since := maxSeq(t, peer)
	setMark(t, peer, "rooms", "rooms-1", "remote", "")
	mustExec(t, peer, `INSERT INTO currencies (id, code, name, symbol, exchange_rate) VALUES ('currencies-peer', 'TSX', 'Clashing Currency', 'A', 3)`)

	_, conflicts := mustApply(t, node, outgoing(t, peer, since))
	if len(conflicts) != 1 || conflicts[0].Resolution != "unique_parked" || conflicts[0].RowID != "currencies-peer" {
		t.Fatalf("conflicts = %+v, want one unique_parked for currencies-peer", conflicts)
	}
	if rowExists(t, node, "currencies", "currencies-peer") {
		t.Error("the parked row was written")
	}
	if got, _ := markOf(t, node, "currencies", "currencies-node"); got != "Node Currency" {
		t.Errorf("currencies-node = %q, want the row that kept its code", got)
	}
	if got, _ := markOf(t, node, "rooms", "rooms-1"); got != "remote" {
		t.Errorf("rooms-1 = %q, want the rest of the batch applied", got)
	}
}

// An incoming row whose parent was deleted here is skipped until the parent
// returns: this node's delete reaches the other node, is refused there (its
// own child), and the parent and the child come back together. Nothing is
// lost on either side.
func TestApply_OrphanSkippedUntilTheParentReturns(t *testing.T) {
	peer, node := newNodePair(t)
	sincePeer := maxSeq(t, peer)
	mustExec(t, peer, `DELETE FROM patients WHERE id = 'patients-2'`)
	sinceNode := maxSeq(t, node)
	mustExec(t, node, `INSERT INTO appointments (id, patient_id, room_id, start_time, end_time, status)
		VALUES ('appointments-node', 'patients-2', 'rooms-1', '2026-02-01T08:00:00Z', '2026-02-01T09:00:00Z', 'Scheduled')`)
	// The child leaves before the delete does, the engine's pull-then-push
	// order when both sides wrote while apart.
	childBatch := outgoing(t, node, sinceNode)
	deleteBatch := outgoing(t, peer, sincePeer)

	// Round one: the deleter skips the child, the holder refuses the delete
	// and re-logs the parent and the child.
	_, conflicts := mustApply(t, peer, childBatch)
	if len(conflicts) != 1 || conflicts[0].Resolution != "orphan_skipped" || conflicts[0].RowID != "appointments-node" {
		t.Fatalf("child on the deleter of the parent: conflicts = %+v, want one orphan_skipped for appointments-node", conflicts)
	}
	_, conflicts = mustApply(t, node, deleteBatch)
	if len(conflicts) != 1 || conflicts[0].Resolution != "delete_refused" || conflicts[0].RowID != "patients-2" {
		t.Fatalf("delete on the holder of the child: conflicts = %+v, want one delete_refused for patients-2", conflicts)
	}

	// Round two: the re-logged parent and child return to the deleter, whose
	// own delete they overtake.
	_, conflicts = mustApply(t, peer, outgoing(t, node, sinceNode))
	if len(conflicts) != 1 || conflicts[0].Resolution != "delete_overtaken" || conflicts[0].RowID != "patients-2" {
		t.Fatalf("conflicts = %+v, want one delete_overtaken for patients-2", conflicts)
	}
	for _, db := range []*sql.DB{peer, node} {
		if !rowExists(t, db, "patients", "patients-2") {
			t.Error("patients-2 is missing on a node")
		}
		if !rowExists(t, db, "appointments", "appointments-node") {
			t.Error("appointments-node is missing on a node")
		}
	}
}

// A local cascade child keeps its parent: the delete of a row that still has
// local children is refused, however the children are linked to it.
func TestApply_CascadeChildKeepsItsParent(t *testing.T) {
	peer, node := newNodePair(t)
	mustExec(t, node, `INSERT INTO patient_allergies (id, patient_id, allergy_id) VALUES ('pal-cascade', 'patients-2', 'allergies-1')`)
	since := maxSeq(t, peer)
	setMark(t, peer, "rooms", "rooms-1", "remote", "")
	mustExec(t, peer, `DELETE FROM patients WHERE id = 'patients-2'`)

	_, conflicts := mustApply(t, node, outgoing(t, peer, since))
	if len(conflicts) != 1 || conflicts[0].Resolution != "delete_refused" || conflicts[0].RowID != "patients-2" {
		t.Fatalf("conflicts = %+v, want one delete_refused for patients-2", conflicts)
	}
	if !rowExists(t, node, "patients", "patients-2") {
		t.Error("patients-2 was deleted")
	}
	if !rowExists(t, node, "patient_allergies", "pal-cascade") {
		t.Error("the cascade child was lost")
	}
	if got, _ := markOf(t, node, "rooms", "rooms-1"); got != "remote" {
		t.Errorf("rooms-1 = %q, want the rest of the batch applied", got)
	}
	if n := count(t, node, `SELECT COUNT(*) FROM sync_log WHERE op = 'update'
		AND ((table_name = 'patients' AND row_id = 'patients-2')
			OR (table_name = 'patient_allergies' AND row_id = 'pal-cascade'))`); n != 2 {
		t.Errorf("re-logged rows in the outbox = %d, want the parent and its cascade child", n)
	}
}

// A row this node deleted and the peer re-created before the delete was
// pushed stays: the stale delete entry is replaced so the row ships as an
// update, the overtaken delete is logged, and the other node keeps the row
// it re-created.
func TestApply_RecreatedRowOvertakesThePendingDelete(t *testing.T) {
	peer, node := newNodePair(t)
	before := maxSeq(t, node)
	mustExec(t, node, `DELETE FROM rooms WHERE id = 'rooms-2'`)
	since := maxSeq(t, peer)
	setMark(t, peer, "rooms", "rooms-2", "peer edit", "")

	// The re-created row arrives (a pull on this node, a push on the other
	// build) and overtakes the pending delete.
	_, conflicts := mustApply(t, node, outgoing(t, peer, since))
	if len(conflicts) != 1 || conflicts[0].Resolution != "delete_overtaken" || conflicts[0].RowID != "rooms-2" {
		t.Fatalf("conflicts = %+v, want one delete_overtaken for rooms-2", conflicts)
	}
	if got, _ := markOf(t, node, "rooms", "rooms-2"); got != "peer edit" {
		t.Errorf("rooms-2 = %q, want the re-created row", got)
	}

	// The outbox ships the row, not the delete, and the row returns cleanly
	// to the node that re-created it.
	batch := outgoing(t, node, before)
	if len(batch) != 1 || batch[0].Table != "rooms" || batch[0].RowID != "rooms-2" ||
		batch[0].Op != "update" || batch[0].RowJSON == nil {
		t.Fatalf("outgoing batch = %+v, want the re-created room as an update", batch)
	}
	if _, conflicts := mustApply(t, peer, batch); len(conflicts) != 0 {
		t.Fatalf("conflicts = %+v, want the row to return cleanly", conflicts)
	}
	if got, _ := markOf(t, peer, "rooms", "rooms-2"); got != "peer edit" {
		t.Errorf("rooms-2 on the peer = %q, want the row it re-created", got)
	}
}

// A delete the receiver refuses keeps its row, and that row keeps its own
// parent: while apart, one node deletes a row and then the row's parent, and
// the other node gives the row a child. The receiver refuses both deletes,
// the batch applies, and the rows go back so both nodes hold them.
func TestApply_RefusedDeleteKeepsItsParent(t *testing.T) {
	cases := []struct {
		name    string
		setup   []string    // on both nodes
		deletes [][2]string // table and id, deleted in this order while apart
		child   string      // the row the other node adds meanwhile
		childAt [2]string   // its table and id
	}{
		{
			name: "room of an appointment given a procedure",
			setup: []string{
				`INSERT INTO rooms (id, name, type) VALUES ('rooms-9', 'Systest Room', 'General')`,
				`INSERT INTO appointments (id, patient_id, room_id, start_time, end_time, status)
					VALUES ('appointments-9', 'patients-1', 'rooms-9', '2026-02-01T08:00:00Z', '2026-02-01T09:00:00Z', 'Scheduled')`,
			},
			deletes: [][2]string{{"appointments", "appointments-9"}, {"rooms", "rooms-9"}},
			child: `INSERT INTO appointment_procedures (id, patient_id, procedure_id, appointment_id)
				VALUES ('appointment_procedures-node', 'patients-1', 'procedures-1', 'appointments-9')`,
			childAt: [2]string{"appointment_procedures", "appointment_procedures-node"},
		},
		{
			name: "patient of a prescription given a medicine line",
			setup: []string{
				`INSERT INTO patients (id, first_name, last_name, date_of_birth, contact) VALUES ('patients-9', 'Systest', 'Patient', '1990-01-01', '555-0109')`,
				`INSERT INTO prescriptions (id, patient_id, prescribed_by_id, start_date) VALUES ('prescriptions-9', 'patients-9', 'employees-1', '2026-02-01')`,
			},
			deletes: [][2]string{{"prescriptions", "prescriptions-9"}, {"patients", "patients-9"}},
			child:   `INSERT INTO prescription_medicines (id, medicine_id, prescription_id) VALUES ('prescription_medicines-node', 'medicines-1', 'prescriptions-9')`,
			childAt: [2]string{"prescription_medicines", "prescription_medicines-node"},
		},
		{
			name: "original of a rescheduled appointment given a procedure",
			setup: []string{
				`INSERT INTO appointments (id, patient_id, room_id, start_time, end_time, status)
					VALUES ('appointments-o', 'patients-1', 'rooms-1', '2026-02-01T08:00:00Z', '2026-02-01T09:00:00Z', 'Rescheduled')`,
				`INSERT INTO appointments (id, patient_id, room_id, start_time, end_time, status, rescheduled_from)
					VALUES ('appointments-n', 'patients-1', 'rooms-1', '2026-02-02T08:00:00Z', '2026-02-02T09:00:00Z', 'Scheduled', 'appointments-o')`,
			},
			deletes: [][2]string{{"appointments", "appointments-n"}, {"appointments", "appointments-o"}},
			child: `INSERT INTO appointment_procedures (id, patient_id, procedure_id, appointment_id)
				VALUES ('appointment_procedures-node', 'patients-1', 'procedures-1', 'appointments-n')`,
			childAt: [2]string{"appointment_procedures", "appointment_procedures-node"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			peer, node := newNodePair(t)
			execAll(t, tc.setup, peer, node)
			down, up := newLink(t, peer, node), newLink(t, node, peer)
			for _, d := range tc.deletes {
				mustExec(t, peer, fmt.Sprintf(`DELETE FROM %q WHERE id = ?`, d[0]), d[1])
			}
			mustExec(t, node, tc.child)

			var want []string
			for _, d := range tc.deletes {
				want = append(want, d[0]+"/"+d[1]+" delete_refused")
			}
			assertResolutions(t, down.ship(t), want...)
			kept := append(append([][2]string{}, tc.deletes...), tc.childAt)
			for _, r := range kept {
				if !rowExists(t, node, r[0], r[1]) {
					t.Errorf("%s/%s was lost on the receiver", r[0], r[1])
				}
			}

			settle(t, down, up)
			for _, r := range kept {
				assertSameRows(t, peer, node, r[0], r[1])
			}
		})
	}
}

// repointCase moves a child from one parent to another on the node that
// then deletes the old parent. Both parents and the child exist on both
// nodes to begin with.
type repointCase struct {
	name                 string
	setup                []string
	parent               string // the parents' table
	oldParent, newParent string
	child, childID       string // the child's table and id
	column               string // the child's reference to its parent
}

var repointCases = []repointCase{
	{
		name: "appointment moved to another room",
		setup: []string{
			`INSERT INTO rooms (id, name, type) VALUES ('rooms-8', 'Systest Old Room', 'General')`,
			`INSERT INTO rooms (id, name, type) VALUES ('rooms-9', 'Systest New Room', 'General')`,
			`INSERT INTO appointments (id, patient_id, room_id, start_time, end_time, status, created_at, updated_at)
				VALUES ('appointments-9', 'patients-1', 'rooms-8', '2026-02-01T08:00:00Z', '2026-02-01T09:00:00Z', 'Scheduled', '` + fixtureTime + `', '` + fixtureTime + `')`,
		},
		parent: "rooms", oldParent: "rooms-8", newParent: "rooms-9",
		child: "appointments", childID: "appointments-9", column: "room_id",
	},
	{
		name: "procedure line handed to another employee",
		setup: []string{
			`INSERT INTO employees (id, first_name, last_name, role, contact, employment_type) VALUES ('employees-8', 'Systest', 'Old', 'Nurse', '555-0108', 'Full-time')`,
			`INSERT INTO employees (id, first_name, last_name, role, contact, employment_type) VALUES ('employees-9', 'Systest', 'New', 'Nurse', '555-0109', 'Full-time')`,
			`INSERT INTO appointment_procedures (id, patient_id, procedure_id, appointment_id, assigned_to_id, created_at, updated_at)
				VALUES ('appointment_procedures-9', 'patients-1', 'procedures-1', 'appointments-1', 'employees-8', '` + fixtureTime + `', '` + fixtureTime + `')`,
		},
		parent: "employees", oldParent: "employees-8", newParent: "employees-9",
		child: "appointment_procedures", childID: "appointment_procedures-9", column: "assigned_to_id",
	},
}

// moveAndDelete moves the child to the new parent on db, stamped at, and
// deletes the old parent there.
func (rc repointCase) moveAndDelete(t *testing.T, db *sql.DB, at string) {
	t.Helper()
	mustExec(t, db, fmt.Sprintf(`UPDATE %q SET %q = ?, updated_at = ? WHERE id = ?`, rc.child, rc.column), rc.newParent, at, rc.childID)
	mustExec(t, db, fmt.Sprintf(`DELETE FROM %q WHERE id = ?`, rc.parent), rc.oldParent)
}

// A child the batch moves to a parent the receiver deleted meanwhile is
// skipped as an orphan, so it still sits under its old parent, which the
// batch deletes: that delete is refused, the batch applies, and the exchange
// that follows leaves both nodes with the same rows.
func TestApply_SkippedMoveKeepsTheOldParent(t *testing.T) {
	for _, rc := range repointCases {
		t.Run(rc.name, func(t *testing.T) {
			peer, node := newNodePair(t)
			execAll(t, rc.setup, peer, node)
			down, up := newLink(t, peer, node), newLink(t, node, peer)
			rc.moveAndDelete(t, peer, "2026-03-01T00:00:00Z")
			mustExec(t, node, fmt.Sprintf(`DELETE FROM %q WHERE id = ?`, rc.parent), rc.newParent)

			assertResolutions(t, down.ship(t),
				rc.child+"/"+rc.childID+" orphan_skipped",
				rc.parent+"/"+rc.oldParent+" delete_refused")
			if got := refOf(t, node, rc.child, rc.childID, rc.column); got != rc.oldParent {
				t.Errorf("%s/%s points at %q on the receiver, want its old parent", rc.child, rc.childID, got)
			}

			settle(t, down, up)
			assertSameRows(t, peer, node, rc.child, rc.childID)
			assertSameRows(t, peer, node, rc.parent, refOf(t, node, rc.child, rc.childID, rc.column))
			for _, id := range []string{rc.oldParent, rc.newParent} {
				if rowExists(t, peer, rc.parent, id) != rowExists(t, node, rc.parent, id) {
					t.Errorf("%s/%s is on one node only", rc.parent, id)
				}
			}
		})
	}
}
