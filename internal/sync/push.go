package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"clinic-api/internal/buildmode"
)

const pushBatchSize = 500

// push sends local sync_log rows after last_pushed_seq.
func (e *Engine) push(ctx context.Context) error {
	lastPushed, _, err := GetState(e.db, SyncedPeer)
	if err != nil {
		return fmt.Errorf("read state: %w", err)
	}

	for {
		batch, err := BuildBatch(e.db, lastPushed, pushBatchSize)
		if err != nil {
			return fmt.Errorf("build batch: %w", err)
		}
		if len(batch.Rows) == 0 {
			return nil
		}

		body, err := json.Marshal(PushRequest{Rows: batch.Rows})
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
		req.Header.Set("X-Sync-Version", buildmode.Version)

		resp, err := e.httpC.Do(req)
		if err != nil {
			return fmt.Errorf("post push: %w", err)
		}
		respRaw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("peer push status %d: %s", resp.StatusCode, string(respRaw))
		}

		ack := batch.LastSeq()
		if err := SetLastPushed(e.db, SyncedPeer, ack); err != nil {
			return fmt.Errorf("set last_pushed: %w", err)
		}
		if _, err := PruneOutgoing(e.db, ack); err != nil {
			return fmt.Errorf("prune outgoing: %w", err)
		}
		lastPushed = ack

		if !batch.Full(pushBatchSize) {
			return nil
		}
	}
}
