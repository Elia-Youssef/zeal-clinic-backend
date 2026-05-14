package sync

import (
	"database/sql"
	"fmt"
)

// GetState loads or creates peer cursors.
func GetState(db *sql.DB, peer string) (lastPushedSeq, lastPulledSeq int64, err error) {
	row := db.QueryRow(
		`SELECT last_pushed_seq, last_pulled_seq FROM sync_state WHERE peer = ?`,
		peer,
	)
	err = row.Scan(&lastPushedSeq, &lastPulledSeq)
	if err == sql.ErrNoRows {
		if _, ierr := db.Exec(
			`INSERT INTO sync_state(peer, last_pushed_seq, last_pulled_seq) VALUES (?,0,0)`,
			peer,
		); ierr != nil {
			return 0, 0, fmt.Errorf("init sync_state: %w", ierr)
		}
		return 0, 0, nil
	}
	return
}

// SetLastPushed advances the acknowledged push cursor.
func SetLastPushed(db *sql.DB, peer string, seq int64) error {
	_, err := db.Exec(
		`UPDATE sync_state SET last_pushed_seq = ?, updated_at = datetime('now') WHERE peer = ?`,
		seq, peer,
	)
	return err
}

// SetLastPulled advances the applied pull cursor.
func SetLastPulled(db *sql.DB, peer string, seq int64) error {
	_, err := db.Exec(
		`UPDATE sync_state SET last_pulled_seq = ?, updated_at = datetime('now') WHERE peer = ?`,
		seq, peer,
	)
	return err
}

// MaxLogSeq returns the largest seq in sync_log, or 0 if empty.
func MaxLogSeq(db *sql.DB) (int64, error) {
	var n sql.NullInt64
	if err := db.QueryRow(`SELECT MAX(seq) FROM sync_log`).Scan(&n); err != nil {
		return 0, err
	}
	if !n.Valid {
		return 0, nil
	}
	return n.Int64, nil
}

// LoadBatch returns sync_log rows after since.
func LoadBatch(db *sql.DB, since int64, limit int) ([]LogEntry, error) {
	rows, err := db.Query(
		`SELECT seq, table_name, row_id, op, created_at
		 FROM sync_log
		 WHERE seq > ?
		 ORDER BY seq ASC
		 LIMIT ?`,
		since, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LogEntry
	for rows.Next() {
		var e LogEntry
		if err := rows.Scan(&e.Seq, &e.Table, &e.RowID, &e.Op, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// PruneOutgoing removes rows acknowledged by the peer.
func PruneOutgoing(db *sql.DB, confirmedSeq int64) (int64, error) {
	if confirmedSeq <= 0 {
		return 0, nil
	}
	res, err := db.Exec(`DELETE FROM sync_log WHERE seq <= ?`, confirmedSeq)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
