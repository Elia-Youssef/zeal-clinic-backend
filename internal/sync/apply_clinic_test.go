//go:build !cloud

package sync_test

import (
	"testing"

	syncpkg "clinic-api/internal/sync"
)

// On the clinic, tables with updated_at keep the local row when it is newer
// (string compare, one-second resolution); ties and older local rows take the
// incoming row.
func TestApplyClinic_NewerRowWinsOnTablesWithUpdatedAt(t *testing.T) {
	tables := tablesWithUpdatedAt(true)
	if len(tables) != 17 {
		t.Fatalf("%d synced tables carry updated_at, want 17", len(tables))
	}
	cases := []struct {
		name                string
		remoteAt, localAt   string
		wantMark            string
		wantLocalWinsLogged bool
	}{
		{"local newer", "2026-02-01T10:00:00Z", "2026-02-01T10:00:01Z", "local", true},
		{"remote newer", "2026-02-01T10:00:01Z", "2026-02-01T10:00:00Z", "remote", false},
		{"same second", "2026-02-01T10:00:00Z", "2026-02-01T10:00:00Z", "remote", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			peer, node := newNodePair(t)
			since := maxSeq(t, peer)
			for _, ti := range tables {
				setMark(t, peer, ti.Name, baseRowID(ti.Name), "remote", tc.remoteAt)
				setMark(t, node, ti.Name, baseRowID(ti.Name), "local", tc.localAt)
			}
			batch := outgoing(t, peer, since)
			if len(batch) != len(tables) {
				t.Fatalf("batch has %d entries, want %d", len(batch), len(tables))
			}

			applied, conflicts := mustApply(t, node, batch)
			if applied != lastSeq(batch) {
				t.Errorf("applied seq = %d, want %d", applied, lastSeq(batch))
			}
			for _, ti := range tables {
				if got, _ := markOf(t, node, ti.Name, baseRowID(ti.Name)); got != tc.wantMark {
					t.Errorf("%s: row = %q, want %q", ti.Name, got, tc.wantMark)
				}
			}
			stored := count(t, node, `SELECT COUNT(*) FROM sync_conflicts WHERE resolution = 'local_wins'`)
			if !tc.wantLocalWinsLogged {
				if len(conflicts) != 0 || stored != 0 {
					t.Fatalf("conflicts = %d returned, %d stored, want none", len(conflicts), stored)
				}
				return
			}
			if len(conflicts) != len(tables) || stored != len(tables) {
				t.Fatalf("conflicts = %d returned, %d stored, want %d", len(conflicts), stored, len(tables))
			}
			for _, c := range conflicts {
				mark := markColumn[c.Table]
				if c.Resolution != "local_wins" || c.RowID != baseRowID(c.Table) ||
					jsonField(t, c.LocalJSON, mark) != "local" || jsonField(t, c.RemoteJSON, mark) != "remote" {
					t.Errorf("conflict = %+v", c)
				}
			}
		})
	}
}

// Tables without updated_at always take the incoming row on the clinic.
func TestApplyClinic_TablesWithoutUpdatedAtTakeTheIncomingRow(t *testing.T) {
	tables := tablesWithUpdatedAt(false)
	if len(tables) != 21 {
		t.Fatalf("%d synced tables lack updated_at, want 21", len(tables))
	}
	peer, node := newNodePair(t)
	since := maxSeq(t, peer)
	for _, ti := range tables {
		setMark(t, peer, ti.Name, baseRowID(ti.Name), "remote", "")
	}
	for _, ti := range tables {
		setMark(t, node, ti.Name, baseRowID(ti.Name), "local", "")
	}
	_, conflicts := mustApply(t, node, outgoing(t, peer, since))
	if len(conflicts) != 0 {
		t.Fatalf("conflicts = %+v, want none", conflicts)
	}
	for _, ti := range tables {
		if got, _ := markOf(t, node, ti.Name, baseRowID(ti.Name)); got != "remote" {
			t.Errorf("%s: row = %q, want the incoming row", ti.Name, got)
		}
	}
}

// A remote delete carries no updated_at, so it wins on the clinic even over
// a local edit made later.
func TestApplyClinic_RemoteDeletesWinOverNewerLocalEdits(t *testing.T) {
	var tables []syncpkg.TableInfo
	for _, ti := range tablesWithUpdatedAt(true) {
		if !ti.NoDelete {
			tables = append(tables, ti)
		}
	}
	if len(tables) != 16 {
		t.Fatalf("%d deletable tables carry updated_at, want 16", len(tables))
	}
	peer, node := newNodePair(t)
	since := maxSeq(t, peer)
	for _, ti := range tables {
		mustExec(t, peer, `DELETE FROM "`+ti.Name+`" WHERE id = ?`, leafRowID(ti.Name))
		setMark(t, node, ti.Name, leafRowID(ti.Name), "local", "2099-01-01T00:00:00Z")
	}
	mustExec(t, peer, `DELETE FROM rooms WHERE id = 'rooms-2'`)
	setMark(t, node, "rooms", "rooms-2", "local", "")

	_, conflicts := mustApply(t, node, outgoing(t, peer, since))
	if len(conflicts) != 0 {
		t.Fatalf("conflicts = %+v, want none", conflicts)
	}
	for _, ti := range tables {
		if rowExists(t, node, ti.Name, leafRowID(ti.Name)) {
			t.Errorf("%s/%s survived the remote delete", ti.Name, leafRowID(ti.Name))
		}
	}
	if rowExists(t, node, "rooms", "rooms-2") {
		t.Error("rooms/rooms-2 survived the remote delete")
	}
}
