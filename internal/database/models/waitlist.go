package models

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type WaitlistEntry struct {
	ID            string `json:"id"`
	PatientID     string `json:"patientId"`
	ProcedureType string `json:"procedureType"`
	PreferredFrom string `json:"preferredFrom"`
	PreferredTo   string `json:"preferredTo"`
	Urgency       string `json:"urgency"`
	Notes         string `json:"notes"`
	Status        string `json:"status"`
	AppointmentID string `json:"appointmentId"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

func (e *WaitlistEntry) GetAll(status string) ([]WaitlistEntry, error) {
	query := `SELECT id, patient_id, procedure_type, preferred_from, preferred_to, urgency, notes, status, appointment_id, created_at, updated_at
		FROM waitlist`
	var args []interface{}
	if status != "" {
		query += " WHERE status = ?"
		args = append(args, status)
	}
	query += " ORDER BY CASE urgency WHEN 'high' THEN 0 WHEN 'normal' THEN 1 WHEN 'low' THEN 2 END, created_at"

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []WaitlistEntry
	for rows.Next() {
		var e WaitlistEntry
		if err := rows.Scan(&e.ID, &e.PatientID, &e.ProcedureType, &e.PreferredFrom, &e.PreferredTo, &e.Urgency, &e.Notes, &e.Status, &e.AppointmentID, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	return list, rows.Err()
}

func (e *WaitlistEntry) Create() error {
	e.ID = uuid.New().String()
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	e.CreatedAt = now
	e.UpdatedAt = now
	if e.Status == "" {
		e.Status = "waiting"
	}
	if e.Urgency == "" {
		e.Urgency = "normal"
	}

	_, err := DB.Exec(`INSERT INTO waitlist (id, patient_id, procedure_type, preferred_from, preferred_to, urgency, notes, status, appointment_id, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		e.ID, e.PatientID, e.ProcedureType, e.PreferredFrom, e.PreferredTo, e.Urgency, e.Notes, e.Status, e.AppointmentID, e.CreatedAt, e.UpdatedAt,
	)
	return err
}

func (e *WaitlistEntry) UpdateStatus(status, appointmentID string) error {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	_, err := DB.Exec(`UPDATE waitlist SET status = ?, appointment_id = ?, updated_at = ? WHERE id = ?`,
		status, appointmentID, now, e.ID)
	if err != nil {
		return err
	}

	err = DB.QueryRow(`SELECT id, patient_id, procedure_type, preferred_from, preferred_to, urgency, notes, status, appointment_id, created_at, updated_at
		FROM waitlist WHERE id = ?`, e.ID).Scan(
		&e.ID, &e.PatientID, &e.ProcedureType, &e.PreferredFrom, &e.PreferredTo, &e.Urgency, &e.Notes, &e.Status, &e.AppointmentID, &e.CreatedAt, &e.UpdatedAt,
	)
	return err
}

func (e *WaitlistEntry) Delete() error {
	res, err := DB.Exec("DELETE FROM waitlist WHERE id = ?", e.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
