package monitor

import (
	"fmt"
	"log"

	"clinic-api/internal/database/store"
	"clinic-api/internal/realtime"
)

// CheckLowStock evaluates the given products against their min_threshold and
// keeps low-stock notifications in sync: products at or below threshold get a
// notification per active user (idempotent via action `low-stock:<productId>`);
// products that recovered above threshold have their prior notifications
// cleared so a subsequent drop re-alerts. Call after any operation that may
// have changed a product's quantity or threshold (invoice create/delete,
// product create/update). Runs asynchronously so the calling request returns
// at its usual speed.
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

	go runLowStockCheck(ids)
}

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
		action := "low-stock:" + pid

		if quantity > threshold {
			res, err := store.DB.Exec(`DELETE FROM notifications WHERE action = ?`, action)
			if err == nil {
				if n, _ := res.RowsAffected(); n > 0 {
					cleared += int(n)
				}
			}
			continue
		}

		var existing int
		if err := store.RDB.QueryRow(`SELECT COUNT(*) FROM notifications WHERE action = ?`, action).Scan(&existing); err != nil || existing > 0 {
			continue
		}

		title := "Low stock: " + name
		desc := fmt.Sprintf("Quantity %d at or below min threshold %d.", quantity, threshold)
		for _, uid := range userIDs {
			n := store.Notification{
				UserID:      uid,
				Title:       title,
				Description: desc,
				Action:      action,
			}
			if err := n.Create(); err == nil {
				sent++
				realtime.SendTo(uid, realtime.Event{Type: "notification", Data: n})
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
