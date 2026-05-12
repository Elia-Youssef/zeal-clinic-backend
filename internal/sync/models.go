package sync

import (
	"encoding/json"
	"strings"
)

// LogEntry is the wire format for one replicated change. The sync_log table
// itself only stores (seq, table_name, row_id, op, created_at); the
// RowJSON and UpdatedAt fields are populated by the push side from the live
// row at the moment of shipping, so the trigger body never has to know what
// columns exist. RowJSON is empty for delete entries.
type LogEntry struct {
	Seq       int64           `json:"seq"`
	Table     string          `json:"table"`
	RowID     string          `json:"row_id"`
	Op        string          `json:"op"`
	RowJSON   json.RawMessage `json:"row_json,omitempty"`
	UpdatedAt string          `json:"updated_at,omitempty"`
	CreatedAt string          `json:"created_at"`
}

// PushRequest carries a batch of changes from the caller to its peer.
type PushRequest struct {
	Rows []LogEntry `json:"rows"`
}

// PushResponse acknowledges what was applied. AppliedSeq is the highest
// caller seq that landed; the caller advances its last_pushed_seq cursor
// to this value. Conflicts is informational: local always wins, but the
// cloud reports them so the caller (the local) can record that its push
// overwrote a peer-side edit.
type PushResponse struct {
	AppliedSeq int64           `json:"applied_seq"`
	Conflicts  []ConflictEntry `json:"conflicts,omitempty"`
}

// PullResponse returns rows from peer where seq > Since. MaxSeq is the
// highest seq present on the peer at the moment of the call, used so the
// caller can detect "we're caught up".
type PullResponse struct {
	Rows   []LogEntry `json:"rows"`
	MaxSeq int64      `json:"max_seq"`
}

// ConflictEntry records that an incoming row was rejected (or downgraded)
// because the receiver's copy was newer, or because the table forbids
// remote deletes. The full payloads are kept for audit.
type ConflictEntry struct {
	Table      string `json:"table"`
	RowID      string `json:"row_id"`
	LocalJSON  string `json:"local_json"`
	RemoteJSON string `json:"remote_json"`
	Resolution string `json:"resolution"`
}

// SyncedPeer is the canonical name of the peer in sync_state for the
// outbound side. Each node has exactly one peer (local <-> cloud), so a
// single fixed key keeps the schema simple.
const SyncedPeer = "peer"

// quoteIdent wraps an identifier in double quotes so SQL keywords used as
// table/column names (e.g. "type") survive parsing.
func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
