package monitor

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"clinic-api/internal/api/middleware"
	"clinic-api/internal/database/store"
	"clinic-api/internal/pdf"
	"clinic-api/internal/realtime"
)

const pdfTTL = 15 * time.Minute

// CleanupPDFCache deletes cached PDFs past the TTL by scanning the tmp dir, so
// files orphaned by a restart are reclaimed too.
func CleanupPDFCache() error {
	dir := pdf.TmpDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	deleted := 0
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".pdf" {
			continue
		}
		info, err := e.Info()
		if err != nil || time.Since(info.ModTime()) < pdfTTL {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			log.Printf("monitor: failed to delete pdf %q: %v", path, err)
			continue
		}
		deleted++
	}
	if deleted > 0 {
		log.Printf("monitor: deleted %d cached pdf(s)", deleted)
	}
	return nil
}

func CleanupExpiredTokens() error {
	return (&store.Token{}).DeleteExpired()
}

func ExpireDiscounts() error {
	res, err := store.DB.Exec(`UPDATE discounts
		SET is_active = 0, updated_at = ?
		WHERE is_active = 1
		  AND end_date IS NOT NULL
		  AND end_date != ''
		  AND end_date < ?`, store.DateNow(), store.ClinicToday())
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("monitor: expired %d discount(s)", n)
		middleware.InvalidateCache("discounts")
		middleware.InvalidateCache("invoice-item-discounts")
	}
	return nil
}

// SendAppointmentReminders notifies all active users about each scheduled
// appointment starting within the next 30 minutes. Idempotency is enforced by
// the action field `appointment-reminder:<appointmentId>`; a reminder is sent
// at most once per appointment across all runs.
func SendAppointmentReminders() error {
	rows, err := store.RDB.Query(`SELECT a.id, COALESCE(p.first_name || ' ' || p.last_name, ''), a.start_time
		FROM appointments a
		LEFT JOIN patients p ON p.id = a.patient_id
		WHERE a.status = 'Scheduled'
		  AND a.start_time >= strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		  AND a.start_time <  strftime('%Y-%m-%dT%H:%M:%SZ', 'now', '+30 minutes')`)
	if err != nil {
		return err
	}
	type reminder struct {
		apptID, patientName, startTime string
	}
	var pending []reminder
	for rows.Next() {
		var r reminder
		if err := rows.Scan(&r.apptID, &r.patientName, &r.startTime); err != nil {
			continue
		}
		pending = append(pending, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(pending) == 0 {
		return nil
	}

	userRows, err := store.RDB.Query(`SELECT id FROM users WHERE is_active = 1`)
	if err != nil {
		return err
	}
	var userIDs []string
	for userRows.Next() {
		var id string
		if err := userRows.Scan(&id); err != nil {
			continue
		}
		userIDs = append(userIDs, id)
	}
	userRows.Close()

	sent := 0
	for _, r := range pending {
		action := "appointment-reminder:" + r.apptID
		var existing int
		if err := store.RDB.QueryRow(`SELECT COUNT(*) FROM notifications WHERE action = ?`, action).Scan(&existing); err != nil || existing > 0 {
			continue
		}

		hm := formatBeirutTime(r.startTime)
		desc := r.patientName
		if hm != "" {
			if r.patientName != "" {
				desc = r.patientName + " at " + hm
			} else {
				desc = "at " + hm
			}
		}

		for _, uid := range userIDs {
			n := store.Notification{
				UserID:      uid,
				Title:       "Upcoming appointment in 30 minutes",
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
		log.Printf("monitor: sent %d appointment reminder notification(s)", sent)
	}
	return nil
}

// formatBeirutTime renders a stored RFC3339 timestamp as HH:MM in the clinic's
// Beirut timezone. Returns "" (and logs) when the value cannot be parsed, so
// callers can fall back to a time-less description rather than silently lying.
func formatBeirutTime(stored string) string {
	t, err := store.Date(stored).Time()
	if err != nil {
		log.Printf("monitor: cannot parse appointment time %q: %v", stored, err)
		return ""
	}
	return t.In(store.ClinicLocation()).Format("15:04")
}

