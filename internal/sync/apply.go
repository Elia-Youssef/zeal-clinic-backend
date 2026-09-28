package sync

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"clinic-api/internal/buildmode"
	"clinic-api/internal/tracking"

	"github.com/google/uuid"
)

// InvalidateCache is injected at startup to avoid a middleware import cycle.
var InvalidateCache func(key string)

// RecalcBalance is injected at startup to avoid a store import cycle.
var RecalcBalance func(tx *sql.Tx, balanceID string) error

// FKViolation names a row whose foreign key points at a missing parent: a
// row written without its parent, or a row left behind by a delete. Apply
// and a cloud restore report it instead of a bare commit failure.
type FKViolation struct {
	Table    string
	RowID    string
	Parent   string
	ParentID string
}

func (v FKViolation) Error() string {
	if v.ParentID == "" {
		return fmt.Sprintf("%s/%s references %s", v.Table, v.RowID, v.Parent)
	}
	return fmt.Sprintf("%s/%s references %s/%s", v.Table, v.RowID, v.Parent, v.ParentID)
}

// batchIndex holds what one Apply pass knows about its batch across both
// loops: the batch's deletes by row, and what apply has decided so far. The
// checks read these decisions, never what the batch asked for: a delete the
// batch sends can be refused, and an upsert can write nothing.
type batchIndex struct {
	deletes  map[string]map[string]LogEntry // table -> the batch's delete entries by row id
	written  map[string]map[string]struct{} // table -> ids of the upserts written
	decided  map[string]map[string]bool     // table -> deletes decided: true when the row was removed, false when it stays
	relogged bool                           // rows went back to the outbox; the peer is woken after the commit
}

func newBatchIndex(deletes map[string][]LogEntry) *batchIndex {
	idx := &batchIndex{
		deletes: make(map[string]map[string]LogEntry),
		written: make(map[string]map[string]struct{}),
		decided: make(map[string]map[string]bool),
	}
	for table, entries := range deletes {
		idx.deletes[table] = make(map[string]LogEntry, len(entries))
		for _, e := range entries {
			idx.deletes[table][e.RowID] = e
		}
	}
	return idx
}

// deletePending reports whether the batch deletes the row and apply has not
// decided that delete yet.
func (idx *batchIndex) deletePending(table, id string) bool {
	if _, deleting := idx.deletes[table][id]; !deleting {
		return false
	}
	_, decided := idx.decided[table][id]
	return !decided
}

// removed reports whether apply carried out the batch's delete of the row.
func (idx *batchIndex) removed(table, id string) bool {
	return idx.decided[table][id]
}

func (idx *batchIndex) decide(table, id string, removed bool) {
	if idx.decided[table] == nil {
		idx.decided[table] = make(map[string]bool)
	}
	idx.decided[table][id] = removed
}

// wrote reports whether apply wrote the batch's upsert of the row; a row it
// kept, parked or skipped wrote nothing.
func (idx *batchIndex) wrote(table, id string) bool {
	_, written := idx.written[table][id]
	return written
}

func (idx *batchIndex) markWritten(table, id string) {
	if idx.written[table] == nil {
		idx.written[table] = make(map[string]struct{})
	}
	idx.written[table][id] = struct{}{}
}

// changes returns how many rows apply wrote or removed, and the tables they
// are in.
func (idx *batchIndex) changes() (int, map[string]struct{}) {
	n := 0
	tables := make(map[string]struct{})
	for table, ids := range idx.written {
		n += len(ids)
		tables[table] = struct{}{}
	}
	for table, ids := range idx.decided {
		for _, removed := range ids {
			if removed {
				n++
				tables[table] = struct{}{}
			}
		}
	}
	return n, tables
}

// ApplyResult is what one Apply did with its batch.
type ApplyResult struct {
	// MaxSeq is the highest outbox seq the batch handled, written or
	// resolved; carried parents (Seq 0) never count.
	MaxSeq int64
	// Written counts the batch's rows apply wrote or removed, renamed or
	// re-created ones included; the rows it kept, parked, skipped or refused
	// changed nothing.
	Written int
	// Conflicts lists every resolution apply recorded, in the order it made
	// them; one row can have two (a refused delete, then a rename).
	Conflicts []ConflictEntry
}

// Apply writes a sync batch in one tx. Local builds use local-wins LWW;
// cloud builds accept incoming rows. NoDelete tables reject deletes.
// Foreign keys are deferred for the batch, so rows may arrive in any order
// (a rescheduled appointment before its original, a child before a parent
// written later), and the batch's own references are probed once before the
// commit; the deferred commit check stays the backstop.
func Apply(db *sql.DB, batch []LogEntry) (res ApplyResult, err error) {
	if len(batch) == 0 {
		return ApplyResult{}, nil
	}

	tx, err := db.Begin()
	if err != nil {
		return ApplyResult{}, fmt.Errorf("begin: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.Exec(`PRAGMA defer_foreign_keys = ON`); err != nil {
		return ApplyResult{}, fmt.Errorf("defer foreign keys: %w", err)
	}
	if _, err = tx.Exec(`UPDATE _sync_applying SET applying = 1 WHERE rowid = 1`); err != nil {
		return ApplyResult{}, fmt.Errorf("raise sync guard: %w", err)
	}

	upserts, deletes, err := planBatch(tx, batch)
	if err != nil {
		return ApplyResult{}, err
	}
	idx := newBatchIndex(deletes)

	// Upserts parents first, deletes children first: tables in SyncedTables
	// order, a table's own rows as planBatch put them.
	for _, t := range SyncedTables {
		for _, e := range upserts[t.Name] {
			entryConflicts, applyErr := applyOneUpsert(tx, idx, t, e)
			if applyErr != nil {
				err = fmt.Errorf("apply %s/%s: %w", e.Table, e.RowID, applyErr)
				return ApplyResult{}, err
			}
			res.Conflicts = append(res.Conflicts, entryConflicts...)
			res.MaxSeq = max(res.MaxSeq, e.Seq)
		}
	}

	for i := len(SyncedTables) - 1; i >= 0; i-- {
		t := SyncedTables[i]
		for _, e := range deletes[t.Name] {
			// A delete no longer pending was decided ahead of an upsert that
			// reused the row's unique value.
			if idx.deletePending(t.Name, e.RowID) {
				c, applyErr := applyDelete(tx, idx, t, e)
				if applyErr != nil {
					err = fmt.Errorf("delete %s/%s: %w", e.Table, e.RowID, applyErr)
					return ApplyResult{}, err
				}
				if c != nil {
					res.Conflicts = append(res.Conflicts, *c)
				}
			}
			res.MaxSeq = max(res.MaxSeq, e.Seq)
		}
	}

	for _, c := range res.Conflicts {
		if _, err = tx.Exec(
			`INSERT INTO sync_conflicts(id, table_name, row_id, local_json, remote_json, resolution) VALUES (?,?,?,?,?,?)`,
			uuid.Must(uuid.NewV7()).String(), c.Table, c.RowID, c.LocalJSON, c.RemoteJSON, c.Resolution,
		); err != nil {
			return ApplyResult{}, fmt.Errorf("log conflict: %w", err)
		}
	}

	if err = recalcBalancesForApplied(tx, upserts["balance_transactions"]); err != nil {
		return ApplyResult{}, err
	}

	if _, err = tx.Exec(`UPDATE _sync_applying SET applying = 0 WHERE rowid = 1`); err != nil {
		return ApplyResult{}, fmt.Errorf("lower sync guard: %w", err)
	}

	if err = checkBatchForeignKeys(tx, idx, upserts, deletes); err != nil {
		return ApplyResult{}, err
	}
	if err = tx.Commit(); err != nil {
		return ApplyResult{}, fmt.Errorf("commit: %w", err)
	}
	if idx.relogged {
		wakePeer()
	}

	written, changed := idx.changes()
	res.Written = written
	reportApplyOutcome(res)
	invalidateCachesFor(changed)
	return res, nil
}

// planBatch groups the batch by table in the order Apply works through it.
// A table's rows keep the batch order, except where the table references
// itself (a rescheduled appointment names its original): its upserts go
// parents first, by the incoming rows, and its deletes children first, by
// the stored rows. With the tables parents first, every parent of a row is
// decided before the row, so an orphan is skipped before anything is
// written against it, and every child of a row before the row's delete.
func planBatch(tx *sql.Tx, batch []LogEntry) (upserts, deletes map[string][]LogEntry, err error) {
	upserts = make(map[string][]LogEntry)
	deletes = make(map[string][]LogEntry)
	for _, e := range batch {
		if e.Op == "delete" {
			deletes[e.Table] = append(deletes[e.Table], e)
		} else {
			upserts[e.Table] = append(upserts[e.Table], e)
		}
	}
	for _, t := range SyncedTables {
		refs, err := parentRefs(tx, t.Name)
		if err != nil {
			return nil, nil, err
		}
		var own []FKRef
		for _, ref := range refs {
			if ref.Parent == t.Name {
				own = append(own, ref)
			}
		}
		if len(own) == 0 {
			continue
		}
		upserts[t.Name] = upsertsParentsFirst(upserts[t.Name], own)
		if deletes[t.Name], err = deletesChildrenFirst(tx, t, deletes[t.Name], own); err != nil {
			return nil, nil, err
		}
	}
	return upserts, deletes, nil
}

// upsertsParentsFirst orders a table's upserts so that a row the batch
// brings comes before the rows of the table that reference it.
func upsertsParentsFirst(entries []LogEntry, own []FKRef) []LogEntry {
	after := make(map[string][]string) // row -> the rows it follows
	for _, e := range entries {
		var row map[string]any
		if json.Unmarshal(e.RowJSON, &row) != nil {
			continue // applyOneUpsert reports the broken row
		}
		for _, ref := range own {
			if parent, ok := fkValue(row, ref.Column); ok {
				after[e.RowID] = append(after[e.RowID], parent)
			}
		}
	}
	return followAfter(entries, after)
}

// deletesChildrenFirst orders a table's deletes so that a row goes before
// the rows of the table it references, as stored.
func deletesChildrenFirst(tx *sql.Tx, t TableInfo, entries []LogEntry, own []FKRef) ([]LogEntry, error) {
	after := make(map[string][]string) // row -> the rows it follows
	for _, e := range entries {
		for _, ref := range own {
			var parent sql.NullString
			err := tx.QueryRow(
				fmt.Sprintf(`SELECT %s FROM %s WHERE %s = ?`,
					quoteIdent(ref.Column), quoteIdent(t.Name), quoteIdent(t.PK())),
				e.RowID,
			).Scan(&parent)
			if err == sql.ErrNoRows {
				break // the row isn't stored; its delete finds nothing
			}
			if err != nil {
				return nil, fmt.Errorf("read %s/%s %s: %w", t.Name, e.RowID, ref.Column, err)
			}
			if parent.Valid {
				after[parent.String] = append(after[parent.String], e.RowID)
			}
		}
	}
	return followAfter(entries, after), nil
}

// followAfter orders entries so that each comes after the entries its row
// follows, keeping the batch order wherever that decides nothing. A cycle of
// rows following each other, which no write makes, is broken where the walk
// closes it.
func followAfter(entries []LogEntry, after map[string][]string) []LogEntry {
	if len(after) == 0 {
		return entries
	}
	pos := make(map[string]int, len(entries))
	for i, e := range entries {
		pos[e.RowID] = i
	}
	seen := make([]bool, len(entries))
	out := make([]LogEntry, 0, len(entries))
	var place func(i int)
	place = func(i int) {
		if seen[i] {
			return
		}
		seen[i] = true // before the rows it follows, so a cycle ends here
		for _, id := range after[entries[i].RowID] {
			if j, ok := pos[id]; ok {
				place(j)
			}
		}
		out = append(out, entries[i])
	}
	for i := range entries {
		place(i)
	}
	return out
}

// relogRows refreshes the outbox entries of rows an apply changed behind the
// sync guard's back, the way the write triggers would: the old entry goes,
// an update entry lands. Called for rows a remote change renamed or kept
// alive, so the other node ends up with the same rows.
func relogRows(tx *sql.Tx, table string, ids ...string) error {
	for _, id := range ids {
		if _, err := tx.Exec(`DELETE FROM sync_log WHERE table_name = ? AND row_id = ?`, table, id); err != nil {
			return fmt.Errorf("clear outbox entry of %s/%s: %w", table, id, err)
		}
		if _, err := tx.Exec(`INSERT INTO sync_log (table_name, row_id, op) VALUES (?, ?, 'update')`, table, id); err != nil {
			return fmt.Errorf("log %s/%s: %w", table, id, err)
		}
	}
	return nil
}

// hexSuffixes returns the stamps an id offers to separate two rows created
// with the same unique value: the last six hex characters of the id, then
// two more at a time up to all of them. Both nodes derive the same list
// whichever of them renames.
func hexSuffixes(id string) []string {
	var hex []byte
	for i := 0; i < len(id); i++ {
		if c := id[i]; (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') {
			hex = append(hex, c)
		}
	}
	var out []string
	for n := 6; n < len(hex); n += 2 {
		out = append(out, string(hex[len(hex)-n:]))
	}
	return append(out, string(hex))
}

// renamedValue returns the value a renamed row takes: the base with the
// shortest stamp from the row's own id that no other row holds, so a value
// typed by hand or a stamp two ids share never clashes again. The renamed
// row goes back out carrying the value chosen, so the nodes converge even
// when they chose differently. "" when every stamp the id offers is taken.
func renamedValue(tx *sql.Tx, t TableInfo, k uniqueKey, base, id string) (string, error) {
	for _, stamp := range hexSuffixes(id) {
		value := base + fmt.Sprintf(k.Suffix, stamp)
		holder, err := uniqueClashRow(tx, t, k, map[string]any{k.Columns[0]: value}, id)
		if err != nil {
			return "", err
		}
		if holder == "" {
			return value, nil
		}
	}
	return "", nil
}

// recalcBalancesForApplied rebuilds balances touched by applied transactions so
// the projection can't drift from source. Missing balance rows are skipped.
func recalcBalancesForApplied(tx *sql.Tx, entries []LogEntry) error {
	if RecalcBalance == nil {
		return nil
	}
	affected := map[string]struct{}{}
	for _, e := range entries {
		var row struct {
			From string `json:"from_balance_id"`
			To   string `json:"to_balance_id"`
		}
		if json.Unmarshal(e.RowJSON, &row) != nil {
			continue
		}
		if row.From != "" {
			affected[row.From] = struct{}{}
		}
		if row.To != "" {
			affected[row.To] = struct{}{}
		}
	}
	for id := range affected {
		if err := RecalcBalance(tx, id); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("recalc balance %s: %w", id, err)
		}
	}
	return nil
}

// reportApplyOutcome fires only when an apply produced conflicts. Successful
// applies happen on every change and would flood logs; errors are tracked
// at the cycle/handler level instead.
func reportApplyOutcome(res ApplyResult) {
	if len(res.Conflicts) == 0 {
		return
	}
	side := "local"
	if buildmode.Cloud {
		side = "cloud"
	}
	tracking.WarnWith(nil,
		fmt.Sprintf("[sync] %s applied %d rows with %d conflicts", side, res.Written, len(res.Conflicts)),
		map[string]any{"conflicts": res.Conflicts},
	)
}

// invalidateCachesFor clears cache buckets affected by applied sync rows.
func invalidateCachesFor(changedTables map[string]struct{}) {
	if InvalidateCache == nil || len(changedTables) == 0 {
		return
	}
	keys := make(map[string]struct{})
	for name := range changedTables {
		t, ok := IsSyncedTable(name)
		if !ok {
			continue
		}
		for _, k := range t.CacheKeys {
			keys[k] = struct{}{}
		}
	}
	for k := range keys {
		InvalidateCache(k)
	}
}

// applyOneUpsert writes one incoming row. A newer local row keeps its place
// (local-wins), a row whose parent is missing everywhere is skipped until
// the parent returns, and a unique value another row already holds is only
// free when the batch deletes that row, in which case that delete is decided
// first. A row that re-creates one this node deleted replaces the stale
// delete entry, so the row goes out, not the delete. A written row is
// recorded in the index. Returns the conflicts the row produced.
func applyOneUpsert(tx *sql.Tx, idx *batchIndex, t TableInfo, e LogEntry) ([]ConflictEntry, error) {
	if len(e.RowJSON) == 0 {
		return nil, fmt.Errorf("empty row_json for %s/%s op=%s", e.Table, e.RowID, e.Op)
	}
	var row map[string]any
	if err := json.Unmarshal(e.RowJSON, &row); err != nil {
		return nil, fmt.Errorf("decode row_json: %w", err)
	}
	if _, ok := row[t.PK()]; !ok {
		return nil, fmt.Errorf("row_json missing %s", t.PK())
	}

	if !buildmode.Cloud && t.HasUpdatedAt {
		keep, localJSON, err := localWinsCheck(tx, t, e)
		if err != nil {
			return nil, err
		}
		if keep {
			return []ConflictEntry{{
				Table:      t.Name,
				RowID:      e.RowID,
				LocalJSON:  localJSON,
				RemoteJSON: string(e.RowJSON),
				Resolution: "local_wins",
			}}, nil
		}
	}

	// A row whose parent is missing waits for its parent: it is skipped, and
	// comes back with the parent once the other node refuses this node's
	// delete of it.
	missing, err := missingParent(tx, t, row)
	if err != nil {
		return nil, err
	}
	if missing {
		return []ConflictEntry{{
			Table:      t.Name,
			RowID:      e.RowID,
			LocalJSON:  "",
			RemoteJSON: string(e.RowJSON),
			Resolution: "orphan_skipped",
		}}, nil
	}

	entryConflicts, parked, err := resolveUniqueClashes(tx, idx, t, e, row)
	if err != nil {
		return nil, err
	}
	if parked {
		return entryConflicts, nil
	}

	// A row this node deleted and the peer re-created before the delete was
	// pushed: the pending delete entry is stale and must not ship as a
	// delete. The row stays, and the overtaken delete is logged.
	var one int
	err = tx.QueryRow(
		`SELECT 1 FROM sync_log WHERE table_name = ? AND row_id = ? AND op = 'delete' LIMIT 1`,
		t.Name, e.RowID,
	).Scan(&one)
	if err == nil {
		if err := relogRows(tx, t.Name, e.RowID); err != nil {
			return nil, err
		}
		idx.relogged = true
		entryConflicts = append(entryConflicts, ConflictEntry{
			Table:      t.Name,
			RowID:      e.RowID,
			LocalJSON:  "",
			RemoteJSON: string(e.RowJSON),
			Resolution: "delete_overtaken",
		})
	} else if err != sql.ErrNoRows {
		return nil, fmt.Errorf("check pending delete of %s/%s: %w", t.Name, e.RowID, err)
	}
	if err := upsert(tx, t, row); err != nil {
		return nil, err
	}
	idx.markWritten(t.Name, e.RowID)
	return entryConflicts, nil
}

// missingParent reports whether a reference of the incoming row names a
// parent this node doesn't hold. Apply decides every parent before its
// children, so a parent the batch brings is stored by now, unless apply
// skipped or parked it.
func missingParent(tx *sql.Tx, t TableInfo, row map[string]any) (bool, error) {
	refs, err := parentRefs(tx, t.Name)
	if err != nil {
		return false, err
	}
	for _, ref := range refs {
		id, ok := fkValue(row, ref.Column)
		if !ok {
			continue
		}
		var one int
		err := tx.QueryRow(
			fmt.Sprintf(`SELECT 1 FROM %s WHERE %s = ? LIMIT 1`,
				quoteIdent(ref.Parent), quoteIdent(tableSet[ref.Parent].PK())),
			id,
		).Scan(&one)
		if err == sql.ErrNoRows {
			return true, nil
		}
		if err != nil {
			return false, fmt.Errorf("check %s parent %s/%s: %w", t.Name, ref.Parent, id, err)
		}
	}
	return false, nil
}

// resolveUniqueClashes gives an incoming row the unique values it carries.
// A value another row already holds is free once the batch's delete of that
// row has run, so that delete is decided here, ahead of the row's own write.
// It refuses like any delete while a row this node keeps references the row
// as the database stands, the batch's later upserts not written yet, and the
// value then stays taken. A clash that stays is resolved by renaming: the
// row with the greater id takes a short suffix from its own id, the same on
// both nodes and lengthened while another row holds the value, and goes back
// out. A clash on a key that can't sensibly be renamed parks the incoming
// row, as does a rename that finds every suffix taken. Returns the
// conflicts, and whether the row was parked.
func resolveUniqueClashes(tx *sql.Tx, idx *batchIndex, t TableInfo, e LogEntry, row map[string]any) ([]ConflictEntry, bool, error) {
	keys, err := uniqueKeys(tx, t.Name)
	if err != nil {
		return nil, false, err
	}
	var conflicts []ConflictEntry
	for _, k := range keys {
		clashID, err := uniqueClashRow(tx, t, k, row, e.RowID)
		if err != nil {
			return nil, false, err
		}
		if clashID == "" {
			continue
		}
		if idx.deletePending(t.Name, clashID) {
			c, applyErr := applyDelete(tx, idx, t, idx.deletes[t.Name][clashID])
			if applyErr != nil {
				return nil, false, fmt.Errorf("delete %s/%s for its unique value: %w", t.Name, clashID, applyErr)
			}
			if c == nil {
				continue // the delete freed the value
			}
			conflicts = append(conflicts, *c)
			// The delete was refused; the clash row keeps its value.
		}

		localBody, _, _, err := fetchLiveRow(tx, t, clashID)
		if err != nil {
			return nil, false, err
		}
		park := ConflictEntry{
			Table:      t.Name,
			RowID:      e.RowID,
			LocalJSON:  string(localBody),
			RemoteJSON: string(e.RowJSON),
			Resolution: "unique_parked",
		}
		if !renamableValue(k, row) {
			return append(conflicts, park), true, nil
		}
		// The row with the greater id takes the suffix, keeps everything
		// else, and goes back out.
		renamed := e.RowID
		if clashID > e.RowID {
			renamed = clashID
		}
		column := k.Columns[0]
		base, _ := row[column].(string)
		newValue, err := renamedValue(tx, t, k, base, renamed)
		if err != nil {
			return nil, false, err
		}
		if newValue == "" {
			return append(conflicts, park), true, nil
		}
		if renamed == clashID {
			// The local row is renamed where it stands.
			if _, err := tx.Exec(
				fmt.Sprintf(`UPDATE %s SET %s = ? WHERE %s = ?`,
					quoteIdent(t.Name), quoteIdent(column), quoteIdent(t.PK())),
				newValue, clashID,
			); err != nil {
				return nil, false, fmt.Errorf("rename %s/%s: %w", t.Name, clashID, err)
			}
		} else {
			// The incoming row takes its new value before its write.
			row[column] = newValue
		}
		if err := relogRows(tx, t.Name, renamed); err != nil {
			return nil, false, err
		}
		idx.relogged = true
		conflicts = append(conflicts, ConflictEntry{
			Table:      t.Name,
			RowID:      renamed,
			LocalJSON:  string(localBody),
			RemoteJSON: string(e.RowJSON),
			Resolution: "unique_renamed",
		})
	}
	return conflicts, false, nil
}

// renamableValue reports whether a clash on the key can be resolved by
// rewriting the value: a single text column on a key marked renamable.
func renamableValue(k uniqueKey, row map[string]any) bool {
	if k.Suffix == "" || len(k.Columns) != 1 {
		return false
	}
	_, isText := row[k.Columns[0]].(string)
	return isText
}

// uniqueClashRow returns the id of a different row holding the key's values,
// or "" when the values are free. NULL in the incoming row never clashes.
func uniqueClashRow(tx *sql.Tx, t TableInfo, k uniqueKey, row map[string]any, rowID string) (string, error) {
	where := make([]string, len(k.Columns))
	args := make([]any, 0, len(k.Columns)+1)
	for i, col := range k.Columns {
		v, present := row[col]
		if !present || v == nil {
			return "", nil
		}
		where[i] = quoteIdent(col) + " = ?"
		args = append(args, v)
	}
	args = append(args, rowID)
	var clashID string
	err := tx.QueryRow(
		fmt.Sprintf(`SELECT %s FROM %s WHERE %s AND %s <> ? LIMIT 1`,
			quoteIdent(t.PK()), quoteIdent(t.Name), strings.Join(where, " AND "), quoteIdent(t.PK())),
		args...,
	).Scan(&clashID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("find %s clash on %s: %w", t.Name, strings.Join(k.Columns, ","), err)
	}
	return clashID, nil
}

// keepers collects the rows this node keeps that reference the row once the
// batch has run, and through them every row referencing those in turn: the
// tree a refused delete sends back, keyed by table. It reads the database,
// which holds the rows the batch has written and none of the ones it has
// removed, so a row found there stays, unless the batch deletes it too,
// apply has not decided that delete yet, and it would run: its table deletes
// rows and no row this node keeps references it (checked the same way).
func keepers(tx *sql.Tx, idx *batchIndex, table, id string) (map[string][]string, error) {
	tree := make(map[string][]string)
	seen := map[string]map[string]struct{}{table: {id: {}}}
	if _, err := collectKeepers(tx, idx, table, id, seen, tree); err != nil {
		return nil, err
	}
	return tree, nil
}

// collectKeepers adds the rows referencing the row that stay, each with its
// own tree, and reports whether any does. A row reached twice in one walk
// counts once.
func collectKeepers(tx *sql.Tx, idx *batchIndex, table, id string, seen map[string]map[string]struct{}, tree map[string][]string) (bool, error) {
	children, err := childRefs(tx, table)
	if err != nil {
		return false, err
	}
	kept := false
	for _, c := range children {
		ids, err := referencingIDs(tx, c, table, id)
		if err != nil {
			return false, err
		}
		for _, childID := range ids {
			if _, visited := seen[c.Table][childID]; visited {
				continue
			}
			if seen[c.Table] == nil {
				seen[c.Table] = make(map[string]struct{})
			}
			seen[c.Table][childID] = struct{}{}

			childKept, err := collectKeepers(tx, idx, c.Table, childID, seen, tree)
			if err != nil {
				return false, err
			}
			if !childKept && idx.deletePending(c.Table, childID) && !tableSet[c.Table].NoDelete {
				continue // the batch's own delete of the row will run
			}
			tree[c.Table] = append(tree[c.Table], childID)
			kept = true
		}
	}
	return kept, nil
}

// referencingIDs returns the ids of the rows whose column c references the
// row id of table.
func referencingIDs(tx *sql.Tx, c FKChild, table, id string) ([]string, error) {
	rows, err := tx.Query(
		fmt.Sprintf(`SELECT %s FROM %s WHERE %s = ?`,
			quoteIdent(tableSet[c.Table].PK()), quoteIdent(c.Table), quoteIdent(c.Column)),
		id,
	)
	if err != nil {
		return nil, fmt.Errorf("find %s referencing %s/%s: %w", c.Table, table, id, err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var childID string
		if err := rows.Scan(&childID); err != nil {
			return nil, fmt.Errorf("scan %s referencing %s/%s: %w", c.Table, table, id, err)
		}
		ids = append(ids, childID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("find %s referencing %s/%s: %w", c.Table, table, id, err)
	}
	return ids, nil
}

// applyDelete decides one of the batch's deletes and records the decision.
// A delete carries no timestamp, so no newer local copy outweighs it: the
// row stays only when its table keeps every row (financial rows, published
// versions), or, since children keep their parent, while a row this node
// keeps still references it once the batch has run. That delete is refused,
// and the row goes back out with the rows that reference it, so the other
// node keeps them too.
func applyDelete(tx *sql.Tx, idx *batchIndex, t TableInfo, e LogEntry) (*ConflictEntry, error) {
	resolution := ""
	var kept map[string][]string
	if t.NoDelete {
		resolution = "no_delete"
	} else {
		var err error
		if kept, err = keepers(tx, idx, t.Name, e.RowID); err != nil {
			return nil, err
		}
		if len(kept) > 0 {
			resolution = "delete_refused"
		}
	}
	if resolution == "" {
		if _, err := tx.Exec(
			fmt.Sprintf(`DELETE FROM %s WHERE %s = ?`, quoteIdent(t.Name), quoteIdent(t.PK())),
			e.RowID,
		); err != nil {
			return nil, err
		}
		idx.decide(t.Name, e.RowID, true)
		return nil, nil
	}

	idx.decide(t.Name, e.RowID, false)
	body, _, _, _ := fetchLiveRow(tx, t, e.RowID)
	if resolution == "delete_refused" {
		kept[t.Name] = append(kept[t.Name], e.RowID)
		for _, st := range SyncedTables {
			if ids := kept[st.Name]; len(ids) > 0 {
				if err := relogRows(tx, st.Name, ids...); err != nil {
					return nil, err
				}
			}
		}
		idx.relogged = true
	}
	return &ConflictEntry{
		Table:      t.Name,
		RowID:      e.RowID,
		LocalJSON:  string(body),
		RemoteJSON: "",
		Resolution: resolution,
	}, nil
}

// localWinsCheck keeps local rows whose updated_at is newer.
func localWinsCheck(tx *sql.Tx, t TableInfo, e LogEntry) (keep bool, localJSON string, err error) {
	var localUpdatedAt sql.NullString
	scanErr := tx.QueryRow(
		fmt.Sprintf(`SELECT IFNULL(updated_at, '') FROM %s WHERE %s = ?`,
			quoteIdent(t.Name), quoteIdent(t.PK())),
		e.RowID,
	).Scan(&localUpdatedAt)
	if scanErr == sql.ErrNoRows {
		return false, "", nil
	}
	if scanErr != nil {
		return false, "", scanErr
	}

	if e.UpdatedAt == "" || localUpdatedAt.String == "" {
		return false, "", nil
	}

	if localUpdatedAt.String > e.UpdatedAt {
		body, _, _, _ := fetchLiveRow(tx, t, e.RowID)
		return true, string(body), nil
	}
	return false, "", nil
}

func upsert(tx *sql.Tx, t TableInfo, row map[string]any) error {
	pk := t.PK()
	keys := sortedKeys(row)
	cols := make([]string, 0, len(keys))
	placeholders := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys))
	setClauses := make([]string, 0, len(keys))
	for _, k := range keys {
		cols = append(cols, quoteIdent(k))
		placeholders = append(placeholders, "?")
		args = append(args, normalizeJSONValue(row[k]))
		if k != pk {
			setClauses = append(setClauses, fmt.Sprintf("%s = excluded.%s", quoteIdent(k), quoteIdent(k)))
		}
	}
	q := fmt.Sprintf(
		`INSERT INTO %s (%s) VALUES (%s) ON CONFLICT(%s) DO UPDATE SET %s`,
		quoteIdent(t.Name),
		strings.Join(cols, ","),
		strings.Join(placeholders, ","),
		quoteIdent(pk),
		strings.Join(setClauses, ","),
	)
	_, err := tx.Exec(q, args...)
	return err
}

// checkBatchForeignKeys probes what the batch changed, before the commit:
// every row it wrote must find its parents, this batch's own writes
// included, and every row it removed must leave no row behind that still
// references it. Rows the batch kept, parked, skipped or refused changed
// nothing and need no probe. A violation names both rows; the deferred
// commit check stays the backstop for anything else.
func checkBatchForeignKeys(tx *sql.Tx, idx *batchIndex, upserts, deletes map[string][]LogEntry) error {
	for _, t := range SyncedTables {
		refs, err := parentRefs(tx, t.Name)
		if err != nil {
			return err
		}
		for _, e := range upserts[t.Name] {
			if !idx.wrote(t.Name, e.RowID) {
				continue
			}
			if err := checkUpsertParents(tx, t, e, refs); err != nil {
				return err
			}
		}
		children, err := childRefs(tx, t.Name)
		if err != nil {
			return err
		}
		for _, e := range deletes[t.Name] {
			if !idx.removed(t.Name, e.RowID) {
				continue
			}
			if err := checkDeleteChildren(tx, t, e.RowID, children); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkDeleteChildren probes a row the batch removed: no row may still
// reference it.
func checkDeleteChildren(tx *sql.Tx, t TableInfo, id string, children []FKChild) error {
	for _, c := range children {
		ids, err := referencingIDs(tx, c, t.Name, id)
		if err != nil {
			return err
		}
		if len(ids) > 0 {
			return FKViolation{Table: c.Table, RowID: ids[0], Parent: t.Name, ParentID: id}
		}
	}
	return nil
}

// checkUpsertParents probes every foreign key of a row the batch wrote, as
// it is stored.
func checkUpsertParents(tx *sql.Tx, t TableInfo, e LogEntry, refs []FKRef) error {
	if len(refs) == 0 {
		return nil
	}
	cols := make([]string, len(refs))
	values := make([]sql.NullString, len(refs))
	scan := make([]any, len(refs))
	for i, ref := range refs {
		cols[i] = quoteIdent(ref.Column)
		scan[i] = &values[i]
	}
	err := tx.QueryRow(
		fmt.Sprintf(`SELECT %s FROM %s WHERE %s = ?`,
			strings.Join(cols, ","), quoteIdent(t.Name), quoteIdent(t.PK())),
		e.RowID,
	).Scan(scan...)
	if err == sql.ErrNoRows {
		return nil // nothing of the row is stored; there is nothing to check
	}
	if err != nil {
		return fmt.Errorf("load %s/%s for its foreign keys: %w", t.Name, e.RowID, err)
	}

	for i, ref := range refs {
		// SQLite exempts only NULL from a foreign key; any other value must
		// name a stored parent.
		if !values[i].Valid {
			continue
		}
		var one int
		err := tx.QueryRow(
			fmt.Sprintf(`SELECT 1 FROM %s WHERE %s = ? LIMIT 1`,
				quoteIdent(ref.Parent), quoteIdent(tableSet[ref.Parent].PK())),
			values[i].String,
		).Scan(&one)
		if err == sql.ErrNoRows {
			return FKViolation{Table: t.Name, RowID: e.RowID, Parent: ref.Parent, ParentID: values[i].String}
		}
		if err != nil {
			return fmt.Errorf("check %s/%s against %s: %w", t.Name, e.RowID, ref.Parent, err)
		}
	}
	return nil
}

// CheckForeignKeys reports the first foreign-key violation in the database,
// naming the offending row and the table it references. A cloud restore runs
// it before committing its replacement of every synced table; an apply uses
// the batch-scoped checkBatchForeignKeys instead. The whole-database check
// can't name the missing parent row, only the child.
func CheckForeignKeys(tx *sql.Tx) error {
	rows, err := tx.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("foreign_key_check: %w", err)
	}
	var table string
	var rowid int64
	var parent string
	var fkid int
	if !rows.Next() {
		err = rows.Err()
		rows.Close()
		return err
	}
	if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
		rows.Close()
		return fmt.Errorf("foreign_key_check: %w", err)
	}
	rows.Close()

	if t, ok := IsSyncedTable(table); ok {
		var id string
		if err := tx.QueryRow(
			fmt.Sprintf(`SELECT %s FROM %s WHERE rowid = ?`, quoteIdent(t.PK()), quoteIdent(table)),
			rowid,
		).Scan(&id); err == nil {
			return FKViolation{Table: table, RowID: id, Parent: parent}
		}
	}
	return fmt.Errorf("%s row %d references %s", table, rowid, parent)
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// normalizeJSONValue converts decoded JSON values for SQLite writes.
func normalizeJSONValue(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case bool:
		if x {
			return int64(1)
		}
		return int64(0)
	case float64:
		if x == float64(int64(x)) {
			return int64(x)
		}
		return x
	case string:
		return x
	default:
		buf, err := json.Marshal(x)
		if err != nil {
			return fmt.Sprintf("%v", x)
		}
		return string(buf)
	}
}
