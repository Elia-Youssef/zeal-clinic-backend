package sync

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// queryer is the read surface *sql.DB and *sql.Tx share, so outbox reads can
// run inside the batch builder's snapshot transaction.
type queryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

// Batch is a push or pull batch built from the outbox: the prefix plus the
// still-pending parents its children need.
type Batch struct {
	// Rows holds the carried parents first, then the outbox prefix in seq
	// order, so the last row is always the prefix's last entry.
	Rows []LogEntry
	// Prefix is the number of outbox entries in Rows; the rows before them
	// are carried parents.
	Prefix int
}

// LastSeq is the seq of the prefix's last entry. It is the only seq a peer
// may ack, cursor or prune on for this batch: carried parents repeat rows the
// outbox still holds, and acknowledging those would prune entries the
// receiver has never seen.
func (b Batch) LastSeq() int64 {
	if len(b.Rows) == 0 {
		return 0
	}
	return b.Rows[len(b.Rows)-1].Seq
}

// Full reports whether the outbox produced a whole prefix of limit entries,
// so more entries may wait behind the batch.
func (b Batch) Full(limit int) bool {
	return b.Prefix >= limit
}

// prefixRows counts the outbox entries in a received batch: carried parents
// (Seq 0) ride along and never fill one.
func prefixRows(rows []LogEntry) int {
	n := 0
	for _, e := range rows {
		if e.Seq > 0 {
			n++
		}
	}
	return n
}

// BuildBatch loads the outbox prefix after since (at most limit entries, in
// seq order) with its live rows, plus every parent those rows still need.
// The outbox keeps one entry per row at its latest write, so a parent
// written after its children sits behind them and a prefix could otherwise
// carry a child whose parent the receiver lacks. Parents whose entry lies
// inside the prefix travel with it anyway, and parents already acknowledged
// are already on the receiver; only entries beyond the prefix end are
// carried, with their own parents, as rows with Seq 0 so no cursor or ack
// can land on them. The batch is read in one transaction, so a row and the
// parents fetched for it share a snapshot.
func BuildBatch(db *sql.DB, since int64, limit int) (Batch, error) {
	tx, err := db.Begin()
	if err != nil {
		return Batch{}, fmt.Errorf("begin batch read: %w", err)
	}
	defer tx.Rollback()

	prefix, err := LoadBatch(tx, since, limit)
	if err != nil {
		return Batch{}, fmt.Errorf("load prefix: %w", err)
	}
	if len(prefix) == 0 {
		return Batch{}, nil
	}
	if err := enrichBatch(tx, prefix); err != nil {
		return Batch{}, err
	}
	deps, err := pendingParents(tx, prefix)
	if err != nil {
		return Batch{}, err
	}

	rows := make([]LogEntry, 0, len(deps)+len(prefix))
	rows = append(rows, deps...)
	rows = append(rows, prefix...)
	if err := tx.Commit(); err != nil {
		return Batch{}, fmt.Errorf("commit batch read: %w", err)
	}
	return Batch{Rows: rows, Prefix: len(prefix)}, nil
}

// pendingParents returns the parents the prefix's upserts need that the
// receiver is still missing: rows whose own outbox entry sits after the
// prefix end and has therefore not been sent. Each parent travels once,
// however many children reference it. The result is ordered parents-first,
// matching SyncedTables.
func pendingParents(tx *sql.Tx, prefix []LogEntry) ([]LogEntry, error) {
	seen := make(map[string]map[string]struct{})
	for _, e := range prefix {
		if seen[e.Table] == nil {
			seen[e.Table] = make(map[string]struct{})
		}
		seen[e.Table][e.RowID] = struct{}{}
	}

	end := prefix[len(prefix)-1].Seq
	var carried []LogEntry
	queue := prefix
	for len(queue) > 0 {
		e := queue[0]
		queue = queue[1:]
		if e.Op == "delete" || len(e.RowJSON) == 0 {
			continue
		}
		var row map[string]any
		if json.Unmarshal(e.RowJSON, &row) != nil {
			continue
		}
		refs, err := parentRefs(tx, e.Table)
		if err != nil {
			return nil, err
		}
		for _, ref := range refs {
			id, ok := fkValue(row, ref.Column)
			if !ok {
				continue
			}
			if seen[ref.Parent] == nil {
				seen[ref.Parent] = make(map[string]struct{})
			}
			if _, done := seen[ref.Parent][id]; done {
				continue
			}
			// Mark every parent, carried or not: a parent that isn't carried
			// is already on the receiver, and its own parents went with it.
			seen[ref.Parent][id] = struct{}{}

			dep, ok, err := carryIfPending(tx, ref.Parent, id, end)
			if err != nil {
				return nil, err
			}
			if ok {
				carried = append(carried, dep)
				queue = append(queue, dep)
			}
		}
	}

	sort.Slice(carried, func(i, j int) bool {
		ri, rj := tableRank(carried[i].Table), tableRank(carried[j].Table)
		if ri != rj {
			return ri < rj
		}
		return carried[i].RowID < carried[j].RowID
	})
	return carried, nil
}

// carryIfPending returns the parent as a dependency row when its outbox
// entry sits beyond after and its live row still exists. The entry ships the
// row now, ahead of its own batch, with Seq 0 so it cannot move a cursor.
// A pending delete never carries: the outbox holds one entry per row at its
// latest write, so a delete entry means the parent row is gone and there is
// no live parent to ship. That holds even when an apply re-created the row
// without logging (the guard is raised): the apply replaces the stale delete
// entry, so a delete entry still standing says the row is gone.
func carryIfPending(tx *sql.Tx, table, id string, after int64) (LogEntry, bool, error) {
	var e LogEntry
	err := tx.QueryRow(
		`SELECT op, created_at FROM sync_log
		 WHERE table_name = ? AND row_id = ? AND seq > ? AND op <> 'delete' LIMIT 1`,
		table, id, after,
	).Scan(&e.Op, &e.CreatedAt)
	if err == sql.ErrNoRows {
		return e, false, nil
	}
	if err != nil {
		return e, false, fmt.Errorf("find pending %s/%s: %w", table, id, err)
	}

	t, _ := IsSyncedTable(table)
	rowJSON, updatedAt, ok, err := fetchLiveRow(tx, t, id)
	if err != nil {
		return e, false, fmt.Errorf("fetch %s/%s: %w", table, id, err)
	}
	if !ok {
		return e, false, nil
	}
	e.Table, e.RowID, e.RowJSON, e.UpdatedAt = table, id, rowJSON, updatedAt
	return e, true, nil
}

// fetchLiveRow reads one live row as an entry body; ok is false when the row
// is gone.
func fetchLiveRow(tx *sql.Tx, t TableInfo, id string) (rowJSON []byte, updatedAt string, ok bool, err error) {
	rows, err := tx.Query(
		fmt.Sprintf(`SELECT * FROM %s WHERE %s = ?`, quoteIdent(t.Name), quoteIdent(t.PK())),
		id,
	)
	if err != nil {
		return nil, "", false, err
	}
	defer rows.Close()
	byPK, err := scanRowsByPK(rows, t.PK())
	if err != nil {
		return nil, "", false, err
	}
	row, present := byPK[id]
	if !present {
		return nil, "", false, nil
	}
	rowJSON, updatedAt, err = rowBody(row)
	if err != nil {
		return nil, "", false, err
	}
	return rowJSON, updatedAt, true, nil
}

// rowBody marshals a scanned row into the body a log entry carries, with its
// updated_at alongside.
func rowBody(row map[string]any) ([]byte, string, error) {
	updatedAt, _ := row["updated_at"].(string)
	body, err := json.Marshal(row)
	if err != nil {
		return nil, "", err
	}
	return body, updatedAt, nil
}

// FKRef is one foreign-key column of a synced table into another synced
// table.
type FKRef struct {
	Column string
	Parent string
}

// fkValue returns the id a row's foreign-key column holds, and whether it
// holds one. Only NULL, or a column the row doesn't carry, references
// nothing: SQLite checks every other value against the parent, the empty
// string included.
func fkValue(row map[string]any, column string) (string, bool) {
	v, present := row[column]
	if !present || v == nil {
		return "", false
	}
	if id, ok := v.(string); ok {
		return id, true
	}
	return fmt.Sprint(v), true
}

// FKChild mirrors FKRef from the parent's side: one column of one synced
// table that references the parent.
type FKChild struct {
	Table  string
	Column string
}

// The schema-derived foreign-key views, read once per process from any
// migrated database and cached only after a full successful read, so a
// failed read is retried with the next batch instead of sticking for the
// process's lifetime.
var (
	fkMu       sync.Mutex
	fkParents  map[string][]FKRef   // child table -> the synced parents it references
	fkChildren map[string][]FKChild // parent table -> the synced tables referencing it
)

// parentRefs returns the foreign-key columns of table that reference another
// synced table.
func parentRefs(db queryer, table string) ([]FKRef, error) {
	parents, _, err := fkViews(db)
	if err != nil {
		return nil, err
	}
	return parents[table], nil
}

// childRefs returns the synced tables that reference table, with the column
// each holds the parent's primary key in.
func childRefs(db queryer, table string) ([]FKChild, error) {
	_, children, err := fkViews(db)
	if err != nil {
		return nil, err
	}
	return children[table], nil
}

// fkViews returns the schema-derived foreign-key views, reading them from
// the schema on first use.
func fkViews(db queryer) (parents map[string][]FKRef, children map[string][]FKChild, err error) {
	fkMu.Lock()
	parents, children = fkParents, fkChildren
	fkMu.Unlock()
	if parents != nil {
		return parents, children, nil
	}
	parents, children, err = readForeignKeys(db)
	if err != nil {
		return nil, nil, err
	}
	fkMu.Lock()
	fkParents, fkChildren = parents, children
	fkMu.Unlock()
	return parents, children, nil
}

// readForeignKeys derives both foreign-key views from the schema: for every
// synced table, the synced tables its foreign-key columns reference, and
// mirrored from that the tables that reference it. A reference counts only
// when it points at the parent's primary key, explicitly or implicitly (an
// FK declared without a column names the primary key): the carry and the
// apply checks look parents up by primary key.
func readForeignKeys(db queryer) (map[string][]FKRef, map[string][]FKChild, error) {
	parents := make(map[string][]FKRef, len(SyncedTables))
	children := make(map[string][]FKChild, len(SyncedTables))
	for _, t := range SyncedTables {
		rows, err := db.Query(`PRAGMA foreign_key_list(` + quoteIdent(t.Name) + `)`)
		if err != nil {
			return nil, nil, fmt.Errorf("read foreign keys of %s: %w", t.Name, err)
		}
		for rows.Next() {
			var id, seq int
			var parent, from, onUpdate, onDelete, match string
			var to sql.NullString
			if err := rows.Scan(&id, &seq, &parent, &from, &to, &onUpdate, &onDelete, &match); err != nil {
				rows.Close()
				return nil, nil, fmt.Errorf("scan foreign key of %s: %w", t.Name, err)
			}
			p, synced := tableSet[parent]
			if !synced {
				continue // sync never reorders rows into an unsynced table
			}
			if to.Valid && to.String != p.PK() {
				return nil, nil, fmt.Errorf("%s.%s references %s.%s, which is not its primary key",
					t.Name, from, parent, to.String)
			}
			parents[t.Name] = append(parents[t.Name], FKRef{Column: from, Parent: parent})
			children[parent] = append(children[parent], FKChild{Table: t.Name, Column: from})
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, nil, fmt.Errorf("read foreign keys of %s: %w", t.Name, err)
		}
		rows.Close()
	}
	return parents, children, nil
}

// tableRank is a table's position in SyncedTables; carried rows are sorted
// parents-first by it.
func tableRank(name string) int {
	for i, t := range SyncedTables {
		if t.Name == name {
			return i
		}
	}
	return len(SyncedTables)
}

// uniqueKey is one secondary unique index of a synced table: the columns two
// rows can hold the same values in, and for a single-column key that can be
// rewritten, the suffix shape a renamed value takes. An empty Suffix marks a
// key that can't sensibly be renamed.
type uniqueKey struct {
	Columns []string
	Suffix  string
}

// renamableKeys lists the single-column unique keys whose value can be
// rewritten when both nodes create the same one, with the suffix shape that
// fits the column's validator. Every other unique key (seed data, compound
// business keys) can't sensibly be renamed, and a clash on it parks the
// incoming row instead.
var renamableKeys = map[string]map[string]string{
	"allergies": {"name": " (%s)"},
	"users":     {"username": "-%s"},
	"discounts": {"code": "-%s"},
}

var (
	uniqueMu  sync.Mutex
	uniqueMap map[string][]uniqueKey // synced table -> its secondary unique keys
)

// uniqueKeys returns the secondary unique indexes of table: the keys another
// row can clash an incoming row's values on.
func uniqueKeys(db queryer, table string) ([]uniqueKey, error) {
	uniqueMu.Lock()
	keys := uniqueMap
	uniqueMu.Unlock()
	if keys == nil {
		var err error
		keys, err = readUniqueKeys(db)
		if err != nil {
			return nil, err
		}
		uniqueMu.Lock()
		uniqueMap = keys
		uniqueMu.Unlock()
	}
	return keys[table], nil
}

// readUniqueKeys derives the secondary unique keys from the schema. Primary
// keys never clash an incoming row (they merge on conflict), and partial and
// expression indexes only hold some rows, so all three stay out.
func readUniqueKeys(db queryer) (map[string][]uniqueKey, error) {
	out := make(map[string][]uniqueKey, len(SyncedTables))
	for _, t := range SyncedTables {
		idx, err := db.Query(`PRAGMA index_list(` + quoteIdent(t.Name) + `)`)
		if err != nil {
			return nil, fmt.Errorf("read indexes of %s: %w", t.Name, err)
		}
		var uniqueIndexes []string
		for idx.Next() {
			var seq int
			var name, origin string
			var unique, partial int
			if err := idx.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
				idx.Close()
				return nil, fmt.Errorf("scan index of %s: %w", t.Name, err)
			}
			if unique == 0 || origin == "pk" || partial != 0 {
				continue
			}
			uniqueIndexes = append(uniqueIndexes, name)
		}
		if err := idx.Err(); err != nil {
			idx.Close()
			return nil, fmt.Errorf("read indexes of %s: %w", t.Name, err)
		}
		idx.Close()

		for _, name := range uniqueIndexes {
			info, err := db.Query(`PRAGMA index_info(` + quoteIdent(name) + `)`)
			if err != nil {
				return nil, fmt.Errorf("read columns of index %s: %w", name, err)
			}
			var cols []string
			for info.Next() {
				var seqno, cid int
				var col sql.NullString
				if err := info.Scan(&seqno, &cid, &col); err != nil {
					info.Close()
					return nil, fmt.Errorf("scan column of index %s: %w", name, err)
				}
				if cid < 0 || !col.Valid {
					cols = nil // an expression index: no column values to compare
					break
				}
				cols = append(cols, col.String)
			}
			if err := info.Err(); err != nil {
				info.Close()
				return nil, fmt.Errorf("read columns of index %s: %w", name, err)
			}
			info.Close()
			if len(cols) > 0 {
				key := uniqueKey{Columns: cols}
				if len(cols) == 1 {
					key.Suffix = renamableKeys[t.Name][cols[0]]
				}
				out[t.Name] = append(out[t.Name], key)
			}
		}
	}
	return out, nil
}

// enrichBatch attaches live row JSON; missing rows become deletes.
func enrichBatch(db queryer, batch []LogEntry) error {
	byTable := make(map[string][]int) // table -> indices into batch needing fetch
	for i, e := range batch {
		if e.Op == "delete" {
			continue
		}
		byTable[e.Table] = append(byTable[e.Table], i)
	}

	for table, idxs := range byTable {
		t, ok := IsSyncedTable(table)
		if !ok {
			continue
		}
		const chunk = 500
		for start := 0; start < len(idxs); start += chunk {
			end := start + chunk
			if end > len(idxs) {
				end = len(idxs)
			}
			window := idxs[start:end]
			ids := make([]any, len(window))
			placeholders := make([]string, len(window))
			for k, i := range window {
				ids[k] = batch[i].RowID
				placeholders[k] = "?"
			}
			rows, err := db.Query(
				fmt.Sprintf(`SELECT * FROM %s WHERE %s IN (%s)`,
					quoteIdent(table), quoteIdent(t.PK()), strings.Join(placeholders, ",")),
				ids...,
			)
			if err != nil {
				return fmt.Errorf("fetch %s: %w", table, err)
			}
			found, err := scanRowsByPK(rows, t.PK())
			rows.Close()
			if err != nil {
				return fmt.Errorf("scan %s: %w", table, err)
			}
			for _, i := range window {
				row, ok := found[batch[i].RowID]
				if !ok {
					batch[i].Op = "delete"
					batch[i].RowJSON = nil
					batch[i].UpdatedAt = ""
					continue
				}
				body, updatedAt, err := rowBody(row)
				if err != nil {
					return fmt.Errorf("marshal row: %w", err)
				}
				batch[i].RowJSON = body
				batch[i].UpdatedAt = updatedAt
			}
		}
	}
	return nil
}

// scanRowsByPK returns rows keyed by pkCol.
func scanRowsByPK(rs *sql.Rows, pkCol string) (map[string]map[string]any, error) {
	cols, err := rs.Columns()
	if err != nil {
		return nil, err
	}
	out := make(map[string]map[string]any)
	for rs.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rs.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(cols))
		var key string
		for i, c := range cols {
			v := vals[i]
			if b, ok := v.([]byte); ok {
				v = string(b)
			}
			row[c] = v
			if c == pkCol {
				if s, ok := v.(string); ok {
					key = s
				}
			}
		}
		if key != "" {
			out[key] = row
		}
	}
	return out, rs.Err()
}
