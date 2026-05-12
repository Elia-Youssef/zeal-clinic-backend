package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"clinic-api/internal/realtime"
)

const pullBatchSize = 500

// pull fetches rows from peer since last_pulled_seq and applies them. Loops
// until the peer reports no more rows. Each batch is one DB transaction.
func (e *Engine) pull(ctx context.Context) error {
	_, lastPulled, err := GetState(e.db, SyncedPeer)
	if err != nil {
		return fmt.Errorf("read state: %w", err)
	}

	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			e.cfg.PeerURL+"/api/sync/pull?since="+strconv.FormatInt(lastPulled, 10)+
				"&limit="+strconv.Itoa(pullBatchSize), nil)
		if err != nil {
			return err
		}
		req.Header.Set("X-Sync-Secret", e.cfg.Secret)

		resp, err := e.httpC.Do(req)
		if err != nil {
			return fmt.Errorf("get pull: %w", err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("peer pull status %d: %s", resp.StatusCode, string(raw))
		}

		var pr PullResponse
		if err := json.Unmarshal(raw, &pr); err != nil {
			return fmt.Errorf("decode pull resp: %w", err)
		}
		if len(pr.Rows) == 0 {
			return nil
		}

		applied, conflicts, err := Apply(e.db, pr.Rows)
		if err != nil {
			return fmt.Errorf("apply pull batch: %w", err)
		}

		// If any rows actually landed (i.e. weren't fully drowned by LWW or
		// no_delete conflicts), tell connected frontends to refetch whatever
		// they're viewing. The event carries no payload; the UI doesn't
		// scope refreshes by table.
		if len(pr.Rows) > len(conflicts) {
			realtime.Broadcast(realtime.Event{Type: "data_changed"})
		}

		// Even if Apply rejected some rows (local-wins), the cursor still
		// advances to the last seq we saw; we don't want to re-pull them.
		cursor := pr.Rows[len(pr.Rows)-1].Seq
		if applied > cursor {
			cursor = applied
		}
		if err := SetLastPulled(e.db, SyncedPeer, cursor); err != nil {
			return fmt.Errorf("set last_pulled: %w", err)
		}
		lastPulled = cursor

		if len(pr.Rows) < pullBatchSize {
			return nil
		}
	}
}
