package monitor

import (
	"log"
	"sync"

	"clinic-api/internal/database/store"
	"clinic-api/internal/realtime"
)

var lowStockWG sync.WaitGroup

// CheckLowStock evaluates the given products against their min_threshold and
// keeps low-stock notifications in sync: products at or below threshold get a
// notification per active user (idempotent via action `low-stock:<productId>`);
// products that recovered above threshold have their prior notifications
// cleared (clearNotices) so a subsequent drop re-alerts. Call after any
// operation that may have changed a product's quantity or threshold (invoice
// create/delete, product create/update). Runs asynchronously so the calling
// request returns at its usual speed.
func CheckLowStock(productIDs []string) {
	if len(productIDs) == 0 {
		return
	}

	seen := make(map[string]struct{}, len(productIDs))
	var ids []string
	for _, id := range productIDs {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return
	}

	lowStockWG.Add(1)
	go func() {
		defer lowStockWG.Done()
		runLowStockCheck(ids)
	}()
}

// WaitAsync waits for low-stock work already launched by completed HTTP
// requests. The restore lifecycle calls it after gating new requests and
// before closing database pools.
func WaitAsync() { lowStockWG.Wait() }

func runLowStockCheck(ids []string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("low-stock: panicked: %v", r)
		}
	}()

	var userIDs []string
	userRows, err := store.RDB.Query(`SELECT id FROM users WHERE is_active = 1`)
	if err != nil {
		log.Printf("low-stock: list users: %v", err)
		return
	}
	for userRows.Next() {
		var id string
		if err := userRows.Scan(&id); err != nil {
			continue
		}
		userIDs = append(userIDs, id)
	}
	userRows.Close()

	sent, cleared := 0, 0
	for _, pid := range ids {
		var name string
		var quantity, threshold int
		if err := store.RDB.QueryRow(`SELECT name, quantity, min_threshold FROM products WHERE id = ?`, pid).
			Scan(&name, &quantity, &threshold); err != nil {
			continue
		}
		notice := store.LowStockNotice(pid, name, quantity, threshold)

		if quantity > threshold {
			cleared += clearNotices(notice.Action)
			continue
		}

		var existing int
		if err := store.RDB.QueryRow(`SELECT COUNT(*) FROM notifications WHERE action = ?`, notice.Action).Scan(&existing); err != nil || existing > 0 {
			continue
		}

		for _, uid := range userIDs {
			notice.UserID = uid
			if err := notice.Create(); err == nil {
				sent++
				realtime.SendTo(uid, realtime.Event{Type: "notification", Data: notice})
			}
		}
	}
	if sent > 0 {
		log.Printf("low-stock: sent %d notification(s)", sent)
	}
	if cleared > 0 {
		log.Printf("low-stock: cleared %d notification(s)", cleared)
	}
}

// clearNotices deletes the notices carrying action and sends each user who
// lost one a NotificationsChanged event, since no other event tells their open
// tabs that a notice is gone. It returns how many notices it deleted.
func clearNotices(action string) int {
	userIDs, err := store.DeleteNotificationsByAction(action)
	if err != nil {
		log.Printf("low-stock: clear %s: %v", action, err)
		return 0
	}
	told := make(map[string]bool, len(userIDs))
	for _, uid := range userIDs {
		if !told[uid] {
			told[uid] = true
			realtime.SendTo(uid, realtime.Event{Type: realtime.NotificationsChanged})
		}
	}
	return len(userIDs)
}
