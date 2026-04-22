package monitor

import (
	"log"
	"time"

	"clinic-api/internal/api/middleware"
	"clinic-api/internal/database/store"
	"clinic-api/internal/realtime"
)

// ExpireDiscounts deactivates discounts whose end_date has passed.
func ExpireDiscounts() error {
	res, err := store.DB.Exec(`UPDATE discounts
		SET is_active = 0, updated_at = ?
		WHERE is_active = 1
		  AND end_date IS NOT NULL
		  AND end_date != ''
		  AND end_date < DATE('now')`, store.DateNow())
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

// ExpirePrescriptionMedicines completes active prescription medicines whose
// parent prescription end_date has passed.
func ExpirePrescriptionMedicines() error {
	res, err := store.DB.Exec(`UPDATE prescription_medicines
		SET status = 'completed'
		WHERE status = 'active'
		  AND prescription_id IN (
		    SELECT id FROM prescriptions
		    WHERE end_date != '' AND end_date < DATE('now')
		  )`)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("monitor: completed %d prescription medicine(s)", n)
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
		  AND datetime(a.start_time) BETWEEN datetime('now') AND datetime('now', '+30 minutes')`)
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

		desc := r.patientName
		if t, err := time.Parse("2006-01-02T15:04:05", r.startTime); err == nil {
			if r.patientName != "" {
				desc = r.patientName + " at " + t.Format("15:04")
			} else {
				desc = "at " + t.Format("15:04")
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
