package sync

import (
	"encoding/json"
	"strings"
)

// LogEntry is one replicated change.
type LogEntry struct {
	// Seq is the entry's outbox sequence; 0 marks a carried parent, which
	// rides along with a batch but is never acked, cursored or pruned.
	Seq       int64           `json:"seq"`
	Table     string          `json:"table"`
	RowID     string          `json:"row_id"`
	Op        string          `json:"op"`
	RowJSON   json.RawMessage `json:"row_json,omitempty"`
	UpdatedAt string          `json:"updated_at,omitempty"`
	CreatedAt string          `json:"created_at"`
}

type PushRequest struct {
	Rows []LogEntry `json:"rows"`
}

type PushResponse struct {
	AppliedSeq int64           `json:"applied_seq"`
	Conflicts  []ConflictEntry `json:"conflicts,omitempty"`
}

type PullResponse struct {
	Rows   []LogEntry `json:"rows"`
	MaxSeq int64      `json:"max_seq"`
}

type ConflictEntry struct {
	Table      string `json:"table"`
	RowID      string `json:"row_id"`
	LocalJSON  string `json:"local_json"`
	RemoteJSON string `json:"remote_json"`
	Resolution string `json:"resolution"`
}

const SyncedPeer = "peer"

func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
