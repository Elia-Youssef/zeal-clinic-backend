//go:build cloud

package sync_test

import (
	"testing"
)

// The cloud takes every incoming row, even over a newer local edit, and
// records no conflicts.
func TestApplyCloud_AcceptsEveryIncomingRow(t *testing.T) {
	peer, node := newNodePair(t)
	since := maxSeq(t, peer)
	for _, r := range fixtureRows {
		if r.id != baseRowID(r.table) {
			continue
		}
		setMark(t, peer, r.table, r.id, "remote", "2026-02-01T10:00:00Z")
		setMark(t, node, r.table, r.id, "local", "2099-01-01T00:00:00Z")
	}
	batch := outgoing(t, peer, since)
	if len(batch) != 38 {
		t.Fatalf("batch has %d entries, want one per synced table (38)", len(batch))
	}

	applied, conflicts := mustApply(t, node, batch)
	if applied != lastSeq(batch) {
		t.Errorf("applied seq = %d, want %d", applied, lastSeq(batch))
	}
	if len(conflicts) != 0 {
		t.Fatalf("conflicts = %+v, want none", conflicts)
	}
	for _, e := range batch {
		if got, _ := markOf(t, node, e.Table, e.RowID); got != "remote" {
			t.Errorf("%s/%s = %q, want the incoming row", e.Table, e.RowID, got)
		}
	}
	if n := count(t, node, `SELECT COUNT(*) FROM sync_conflicts`); n != 0 {
		t.Errorf("sync_conflicts has %d rows", n)
	}
}

// Deletes replicate to the cloud, except on the no-delete tables.
func TestApplyCloud_AppliesDeletes(t *testing.T) {
	peer, node := newNodePair(t)
	since := maxSeq(t, peer)
	var deleted [][2]string
	for _, r := range fixtureRows {
		if r.id != leafRowID(r.table) || r.table == "invoices" || r.table == "invoice_items" ||
			r.table == "balance_transactions" || r.table == "versions" {
			continue
		}
		mustExec(t, peer, `DELETE FROM "`+r.table+`" WHERE id = ?`, r.id)
		setMark(t, node, r.table, r.id, "local", "2099-01-01T00:00:00Z")
		deleted = append(deleted, [2]string{r.table, r.id})
	}
	if len(deleted) != 17 {
		t.Fatalf("deleted %d leaf rows, want 17", len(deleted))
	}
	_, conflicts := mustApply(t, node, outgoing(t, peer, since))
	if len(conflicts) != 0 {
		t.Fatalf("conflicts = %+v, want none", conflicts)
	}
	for _, d := range deleted {
		if rowExists(t, node, d[0], d[1]) {
			t.Errorf("%s/%s survived the delete", d[0], d[1])
		}
	}
}
