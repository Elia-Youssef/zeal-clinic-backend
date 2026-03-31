package models

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

type Appointment struct {
	ID                        string  `json:"id"`
	PatientID                 string  `json:"patientId"`
	RoomID                    string  `json:"roomId"`
	EmployeeID                string  `json:"employeeId"`
	PatientProcedureSessionID *string `json:"patientProcedureSessionId"`
	StartTime                 Date    `json:"startTime"`
	EndTime                   Date    `json:"endTime"`
	Status                    string  `json:"status"`
	Notes                     string  `json:"notes"`
	CreatedAt                 Date    `json:"createdAt"`
	UpdatedAt                 Date    `json:"updatedAt"`
}

func (a *Appointment) IsValid() error {
	e := make(validation.Errors)
	if msg := validation.Required(a.PatientID, "Patient ID"); msg != "" {
		e["patientId"] = msg
	}
	if msg := validation.Required(a.RoomID, "Room ID"); msg != "" {
		e["roomId"] = msg
	}
	if msg := validation.Required(string(a.StartTime), "Start time"); msg != "" {
		e["startTime"] = msg
	} else if msg := validation.DateTime(string(a.StartTime)); msg != "" {
		e["startTime"] = msg
	}
	if msg := validation.Required(string(a.EndTime), "End time"); msg != "" {
		e["endTime"] = msg
	} else if msg := validation.DateTime(string(a.EndTime)); msg != "" {
		e["endTime"] = msg
	}
	if msg := validation.OneOf(a.Status, []string{"Scheduled", "In-Progress", "Completed", "Cancelled"}, "Status"); msg != "" {
		e["status"] = msg
	}
	if len(e) > 0 {
		return e
	}
	return nil
}

const appointmentColumnsNoId = `patient_id, room_id, employee_id, patient_procedure_session_id,
	start_time, end_time, status, notes, created_at, updated_at`
const appointmentColumns = `id, ` + appointmentColumnsNoId

type AppointmentList []Appointment

func (a *Appointment) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil appointment row")
	}
	err := row.Scan(&a.ID, &a.PatientID, &a.RoomID, &a.EmployeeID, &a.PatientProcedureSessionID,
		&a.StartTime, &a.EndTime, &a.Status,
		&a.Notes, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return err
	}
	return nil
}

func (l *AppointmentList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil appointment rows")
	}
	*l = AppointmentList{}
	for rows.Next() {
		var item Appointment
		err := rows.Scan(&item.ID, &item.PatientID, &item.RoomID, &item.EmployeeID, &item.PatientProcedureSessionID,
			&item.StartTime, &item.EndTime, &item.Status,
			&item.Notes, &item.CreatedAt, &item.UpdatedAt)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func checkAppointmentConflict(roomID string, startTime, endTime Date, excludeID string) error {
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

	var list AppointmentList
	if err := list.ScanRows(rows); err != nil {
		return nil, err
	}
	return list, rows.Err()
}

func (a *Appointment) GetByID(id string) error {
	row := DB.QueryRow(`SELECT `+appointmentColumns+` FROM appointments WHERE id = ?`, id)
	return a.ScanRow(row)
}

func (a *Appointment) Create() error {
	if err := checkAppointmentConflict(a.RoomID, a.StartTime, a.EndTime, ""); err != nil {
		return err
	}

	a.ID = uuid.Must(uuid.NewV7()).String()
	now := DateNow()
	a.CreatedAt = now
	a.UpdatedAt = now

	_, err := DB.Exec(`INSERT INTO appointments (`+appointmentColumns+`)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		a.ID, a.PatientID, a.RoomID, a.EmployeeID, a.PatientProcedureSessionID,
		a.StartTime, a.EndTime, a.Status,
		a.Notes, a.CreatedAt, a.UpdatedAt)
	return err
}

func (a *Appointment) Update(updates map[string]interface{}) error {
	cols := map[string]string{
		"patientId": "patient_id", "roomId": "room_id", "employeeId": "employee_id",
		"patientProcedureSessionId": "patient_procedure_session_id",
		"startTime": "start_time", "endTime": "end_time",
		"status": "status", "notes": "notes",
	}

	setClauses := ""
	var args []interface{}
	for jsonKey, dbCol := range cols {
		if val, ok := updates[jsonKey]; ok {
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
			startTime = Date(v)
		}
		if v, ok := updates["endTime"].(string); ok {
			endTime = Date(v)
		}
		if err := checkAppointmentConflict(roomID, startTime, endTime, a.ID); err != nil {
			return err
		}
	}

	setClauses += ", updated_at = ?"
	args = append(args, DateNow())
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
