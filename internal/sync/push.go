package sync

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const pushBatchSize = 500

// push ships local sync_log rows (seq > last_pushed_seq) to the peer in
// batches. Triggers collapse the log in place (only the latest entry per
// (table, row_id) ever sits there), so push is straight: load, enrich
// with live row state, POST, ACK, prune.
func (e *Engine) push(ctx context.Context) error {
	lastPushed, _, err := GetState(e.db, SyncedPeer)
	if err != nil {
		return fmt.Errorf("read state: %w", err)
	}

	for {
		batch, err := LoadBatch(e.db, lastPushed, pushBatchSize)
		if err != nil {
			return fmt.Errorf("load batch: %w", err)
		}
		if len(batch) == 0 {
			return nil
		}

		if err := enrichBatch(e.db, batch); err != nil {
			return fmt.Errorf("enrich batch: %w", err)
		}

		body, err := json.Marshal(PushRequest{Rows: batch})
		if err != nil {
			return fmt.Errorf("marshal push: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			e.cfg.PeerURL+"/api/sync/push", bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Sync-Secret", e.cfg.Secret)

		resp, err := e.httpC.Do(req)
		if err != nil {
			return fmt.Errorf("post push: %w", err)
		}
		respRaw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("peer push status %d: %s", resp.StatusCode, string(respRaw))
		}

		ack := batch[len(batch)-1].Seq
		if err := SetLastPushed(e.db, SyncedPeer, ack); err != nil {
			return fmt.Errorf("set last_pushed: %w", err)
		}
		if _, err := PruneOutgoing(e.db, ack); err != nil {
			return fmt.Errorf("prune outgoing: %w", err)
		}
		lastPushed = ack

		if len(batch) < pushBatchSize {
			return nil
		}
	}
}

// enrichBatch populates RowJSON and UpdatedAt on each non-delete entry by
// reading the current row from its source table. Entries whose row is gone
// (deleted between trigger fire and push) are downgraded to op='delete':
// shipping the live state means an absent row is, semantically, a delete.
//
// Issues one SELECT per table, batching ids into IN(...) lists capped at
// 500 so we don't blow SQLite's parameter limit.
func enrichBatch(db *sql.DB, batch []LogEntry) error {
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
			// Shouldn't happen: only triggers write to sync_log, and they
			// only fire on synced tables. Skip defensively.
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
					// Row was deleted between trigger fire and fetch.
					batch[i].Op = "delete"
					batch[i].RowJSON = nil
					batch[i].UpdatedAt = ""
					continue
				}
				if ua, ok := row["updated_at"].(string); ok {
					batch[i].UpdatedAt = ua
				}
				buf, err := json.Marshal(row)
				if err != nil {
					return fmt.Errorf("marshal row: %w", err)
				}
				batch[i].RowJSON = buf
			}
		}
	}
	return nil
}

// scanRowsByPK reads every row from rs into a map keyed by the value of
// pkCol. Column types are preserved as the driver returns them so the wire
// JSON matches what the receiver will see when it applies via
// INSERT ... ON CONFLICT.
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
			// SQLite returns []byte for TEXT/BLOB; normalise TEXT to string so
			// json.Marshal doesn't base64-encode it.
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
