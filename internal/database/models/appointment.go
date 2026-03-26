package models

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Appointment struct {
	ID                        string `json:"id"`
	PatientID                 string `json:"patientId"`
	RoomID                    string `json:"roomId"`
	EmployeeID                string `json:"employeeId"`
	PatientProcedureSessionID string `json:"patientProcedureSessionId"`
	StartTime                 string `json:"startTime"`
	EndTime                   string `json:"endTime"`
	TreatmentType             string `json:"treatmentType"`
	Status                    string `json:"status"`
	ApprovalStatus            string `json:"approvalStatus"`
	ApprovedBy                string `json:"approvedBy"`
	ApprovedAt                string `json:"approvedAt"`
	Notes                     string `json:"notes"`
	ReminderSent              bool   `json:"reminderSent"`
	ReminderSentAt            string `json:"reminderSentAt"`
	CreatedAt                 string `json:"createdAt"`
	UpdatedAt                 string `json:"updatedAt"`
}

const appointmentColumns = `id, patient_id, room_id, employee_id, patient_procedure_session_id,
	start_time, end_time, treatment_type, status, approval_status, approved_by, approved_at,
	notes, reminder_sent, reminder_sent_at, created_at, updated_at`

func checkAppointmentConflict(roomID, startTime, endTime, excludeID string) error {
	query := `SELECT COUNT(*) FROM appointments
		WHERE room_id = ?
		AND status != 'Cancelled'
		AND start_time < ?
		AND end_time > ?`
	args := []interface{}{roomID, endTime, startTime}
	if excludeID != "" {
		query += " AND id != ?"
		args = append(args, excludeID)
	}
	var count int
	if err := DB.QueryRow(query, args...).Scan(&count); err != nil {
		return fmt.Errorf("check conflict: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("room conflict: another appointment is already booked in this room during the requested time")
	}
	return nil
}

func (a *Appointment) GetAll() ([]Appointment, error) {
	rows, err := DB.Query(`SELECT ` + appointmentColumns + ` FROM appointments ORDER BY start_time`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var apts []Appointment
	for rows.Next() {
		apt, err := scanAppointment(rows)
		if err != nil {
			return nil, err
		}
		apts = append(apts, apt)
	}
	return apts, rows.Err()
}

func (a *Appointment) GetByID(id string) error {
	row := DB.QueryRow(`SELECT `+appointmentColumns+` FROM appointments WHERE id = ?`, id)
	result, err := scanAppointmentRow(row)
	if err != nil {
		return err
	}
	*a = result
	return nil
}

func (a *Appointment) Create() error {
	if err := checkAppointmentConflict(a.RoomID, a.StartTime, a.EndTime, ""); err != nil {
		return err
	}

	a.ID = uuid.New().String()
	now := time.Now().Format(time.RFC3339)
	a.CreatedAt = now
	a.UpdatedAt = now
	if a.ApprovalStatus == "" {
		a.ApprovalStatus = "not_required"
	}

	_, err := DB.Exec(`INSERT INTO appointments (`+appointmentColumns+`)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		a.ID, a.PatientID, a.RoomID, a.EmployeeID, a.PatientProcedureSessionID,
		a.StartTime, a.EndTime, a.TreatmentType, a.Status, a.ApprovalStatus, a.ApprovedBy, a.ApprovedAt,
		a.Notes, BoolToInt(a.ReminderSent), a.ReminderSentAt, a.CreatedAt, a.UpdatedAt)
	return err
}

func (a *Appointment) Update(updates map[string]interface{}) error {
	cols := map[string]string{
		"patientId": "patient_id", "roomId": "room_id", "employeeId": "employee_id",
		"patientProcedureSessionId": "patient_procedure_session_id",
		"startTime": "start_time", "endTime": "end_time",
		"treatmentType": "treatment_type", "status": "status",
		"approvalStatus": "approval_status", "approvedBy": "approved_by", "approvedAt": "approved_at",
		"notes": "notes", "reminderSent": "reminder_sent", "reminderSentAt": "reminder_sent_at",
	}

	setClauses := ""
	var args []interface{}
	for jsonKey, dbCol := range cols {
		if val, ok := updates[jsonKey]; ok {
			if dbCol == "reminder_sent" {
				if b, ok := val.(bool); ok {
					val = BoolToInt(b)
				}
			}
			if setClauses != "" {
				setClauses += ", "
			}
			setClauses += dbCol + " = ?"
			args = append(args, val)
		}
	}
	if setClauses == "" {
		return a.GetByID(a.ID)
	}

	_, roomChanged := updates["roomId"]
	_, startChanged := updates["startTime"]
	_, endChanged := updates["endTime"]
	if roomChanged || startChanged || endChanged {
		var current Appointment
		if err := current.GetByID(a.ID); err != nil {
			return err
		}
		roomID := current.RoomID
		startTime := current.StartTime
		endTime := current.EndTime
		if v, ok := updates["roomId"].(string); ok {
			roomID = v
		}
		if v, ok := updates["startTime"].(string); ok {
			startTime = v
		}
		if v, ok := updates["endTime"].(string); ok {
			endTime = v
		}
		if err := checkAppointmentConflict(roomID, startTime, endTime, a.ID); err != nil {
			return err
		}
	}

	setClauses += ", updated_at = ?"
	args = append(args, time.Now().Format(time.RFC3339))
	args = append(args, a.ID)
	_, err := DB.Exec("UPDATE appointments SET "+setClauses+" WHERE id = ?", args...)
	if err != nil {
		return err
	}
	return a.GetByID(a.ID)
}

func (a *Appointment) Delete() error {
	res, err := DB.Exec("DELETE FROM appointments WHERE id = ?", a.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (a *Appointment) MarkReminded(sent bool) error {
	sentVal := 0
	sentAt := ""
	if sent {
		sentVal = 1
		sentAt = time.Now().UTC().Format("2006-01-02 15:04:05")
	}
	res, err := DB.Exec(`UPDATE appointments SET reminder_sent = ?, reminder_sent_at = ?, updated_at = ? WHERE id = ?`,
		sentVal, sentAt, time.Now().Format(time.RFC3339), a.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (a *Appointment) Approve(approvedBy string) error {
	now := time.Now().Format(time.RFC3339)
	_, err := DB.Exec(`UPDATE appointments SET approval_status = 'approved', approved_by = ?, approved_at = ?, updated_at = ? WHERE id = ?`,
		approvedBy, now, now, a.ID)
	if err != nil {
		return err
	}
	return a.GetByID(a.ID)
}

func (a *Appointment) Reject(approvedBy string) error {
	now := time.Now().Format(time.RFC3339)
	_, err := DB.Exec(`UPDATE appointments SET approval_status = 'rejected', approved_by = ?, approved_at = ?, updated_at = ? WHERE id = ?`,
		approvedBy, now, now, a.ID)
	if err != nil {
		return err
	}
	return a.GetByID(a.ID)
}

func scanAppointmentFields(s scannable) (Appointment, error) {
	var a Appointment
	var reminderSent int
	err := s.Scan(&a.ID, &a.PatientID, &a.RoomID, &a.EmployeeID, &a.PatientProcedureSessionID,
		&a.StartTime, &a.EndTime, &a.TreatmentType, &a.Status, &a.ApprovalStatus, &a.ApprovedBy, &a.ApprovedAt,
		&a.Notes, &reminderSent, &a.ReminderSentAt, &a.CreatedAt, &a.UpdatedAt)
	a.ReminderSent = reminderSent == 1
	return a, err
}

func scanAppointment(rows *sql.Rows) (Appointment, error) {
	return scanAppointmentFields(rows)
}

func scanAppointmentRow(row *sql.Row) (Appointment, error) {
	return scanAppointmentFields(row)
}
