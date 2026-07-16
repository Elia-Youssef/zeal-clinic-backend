package sync

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"clinic-api/internal/buildmode"
	"clinic-api/internal/realtime"
	"clinic-api/internal/tracking"

	"github.com/google/uuid"
)

const syncFailureAction = "sync-failure"

var syncFailureNotifyMu sync.Mutex
var syncFailureEpisodeActive bool

// notifySyncFailure creates notifications only when syncing transitions into a
// failed episode. Repeated retries during the same outage do not recreate a
// notification that a user has read or deleted.
func notifySyncFailure(db *sql.DB) {
	if db == nil {
		return
	}
	syncFailureNotifyMu.Lock()
	defer syncFailureNotifyMu.Unlock()
	if syncFailureEpisodeActive {
		return
	}

	rows, err := db.Query(`SELECT id FROM users WHERE is_active = 1 AND NOT EXISTS (
		SELECT 1 FROM notifications n
		WHERE n.user_id = users.id AND n.action = ? AND n.is_read = 0
	)`, syncFailureAction)
	if err != nil {
		log.Printf("[sync] notify failure: load users: %v", err)
		return
	}
	var userIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			userIDs = append(userIDs, id)
		}
	}
	rows.Close()
	syncFailureEpisodeActive = true

	const title = "Data sync failed"
	const description = "Data synchronization failed. Critical cloud actions are disabled until syncing recovers."
	createdAt := time.Now().UTC().Format(time.RFC3339)
	for _, userID := range userIDs {
		id := uuid.Must(uuid.NewV7()).String()
		if _, err := db.Exec(`INSERT INTO notifications
			(id, user_id, title, description, action, is_read, created_at)
			VALUES (?, ?, ?, ?, ?, 0, ?)`,
			id, userID, title, description, syncFailureAction, createdAt); err != nil {
			log.Printf("[sync] notify failure: user %s: %v", userID, err)
			continue
		}
		realtime.SendTo(userID, realtime.Event{Type: "notification", Data: map[string]any{
			"id": id, "userId": userID, "title": title, "description": description,
			"action": syncFailureAction, "isRead": false, "createdAt": createdAt,
		}})
	}
}

// markSyncRecovered re-arms failure notifications after a complete successful
// pull/push/ready cycle. A later failure is then a new episode.
func markSyncRecovered() {
	syncFailureNotifyMu.Lock()
	syncFailureEpisodeActive = false
	syncFailureNotifyMu.Unlock()
}

// reportFailureToPeer closes the cloud gate for the current SSE session. It is
// best-effort because the same network failure may make the peer unreachable.
func (e *Engine) reportFailureToPeer(ctx context.Context) {
	e.sessionMu.RLock()
	token := e.sessionToken
	e.sessionMu.RUnlock()
	if token == "" {
		return
	}
	body, _ := json.Marshal(readyRequest{SessionToken: token})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.cfg.PeerURL+"/api/sync/failed", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Sync-Secret", e.cfg.Secret)
	req.Header.Set("X-Sync-Version", buildmode.Version)
	resp, err := e.httpC.Do(req)
	if err != nil {
		log.Printf("[sync] report failure to peer: %v", err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
		log.Printf("[sync] report failure to peer: status %d", resp.StatusCode)
	}
}

func (e *Engine) handleCycleFailure(ctx context.Context, stage string, err error) {
	wrapped := fmt.Errorf("[sync] %s: %w", stage, err)
	log.Printf("%v", wrapped)
	tracking.CaptureError(nil, wrapped)
	notifySyncFailure(e.db)
	e.reportFailureToPeer(ctx)
}
