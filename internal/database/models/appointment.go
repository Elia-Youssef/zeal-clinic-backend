package models

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Appointment struct {
	ID        string `json:"id"`
	PatientID string `json:"patientId"`
	RoomID    string `json:"roomId"`
	StartTime Date   `json:"startTime"`
	EndTime   Date   `json:"endTime"`
	Status    string `json:"status"`
	Notes     string `json:"notes"`
	CreatedAt Date   `json:"createdAt"`
	UpdatedAt Date   `json:"updatedAt"`
	// Transient fields (not stored in appointments table)
	ProcedureID        *string `json:"procedureId,omitempty"`
	ProcedureSessionID *string `json:"procedureSessionId,omitempty"`
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

const appointmentColumnsNoId = `patient_id, room_id, start_time, end_time, status, notes, created_at, updated_at`
const appointmentColumns = `id, ` + appointmentColumnsNoId

type AppointmentList []Appointment

func (a *Appointment) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil appointment row")
	}
	err := row.Scan(&a.ID, &a.PatientID, &a.RoomID,
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
		err := rows.Scan(&item.ID, &item.PatientID, &item.RoomID,
			&item.StartTime, &item.EndTime, &item.Status,
			&item.Notes, &item.CreatedAt, &item.UpdatedAt)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func checkAppointmentConflict(db DBTX, roomID string, startTime, endTime Date, excludeID string) error {
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
	if err := db.QueryRow(query, args...).Scan(&count); err != nil {
		return fmt.Errorf("check conflict: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("room conflict: another appointment is already booked in this room during the requested time")
	}
	return nil
}

func (a *AppointmentList) GetAll(date string, params ListParams) (int, error) {
	where := " WHERE DATE(start_time) = ?"
	args := []interface{}{date}
	if fc, fa := params.FilterClause("status", "notes"); fc != "" {
		where += " AND " + fc
		args = append(args, fa...)
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*) FROM appointments"+where, args...).Scan(&total); err != nil {
		return 0, err
	}

	query := `SELECT ` + appointmentColumns + ` FROM appointments` + where + ` ORDER BY start_time` + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	if err := a.ScanRows(rows); err != nil {
		return 0, err
	}
	return total, rows.Err()
}

type RoomDayCount struct {
	RoomID   string         `json:"roomId"`
	RoomName string         `json:"roomName"`
	Days     map[string]int `json:"days"`
}

func GetAppointmentCountPerRoom(date string) ([]RoomDayCount, error) {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return nil, fmt.Errorf("invalid date format: %w", err)
	}

	weekday := t.Weekday()
	offset := int(weekday - time.Monday)
	if offset < 0 {
		offset = 6
	}
	weekStart := t.AddDate(0, 0, -offset)
	weekEnd := weekStart.AddDate(0, 0, 6)

	query := `SELECT r.id, r.name, DATE(a.start_time) as day, COUNT(a.id) as count
		FROM rooms r
		LEFT JOIN appointments a ON a.room_id = r.id
			AND DATE(a.start_time) BETWEEN ? AND ?
			AND a.status != 'Cancelled'
		GROUP BY r.id, r.name, DATE(a.start_time)
		ORDER BY r.name, day`
	rows, err := RDB.Query(query, weekStart.Format("2006-01-02"), weekEnd.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	roomMap := make(map[string]*RoomDayCount)
	var order []string
	for rows.Next() {
		var roomID, roomName string
		var day sql.NullString
		var count int
		if err := rows.Scan(&roomID, &roomName, &day, &count); err != nil {
			continue
		}
		if _, exists := roomMap[roomID]; !exists {
			roomMap[roomID] = &RoomDayCount{RoomID: roomID, RoomName: roomName, Days: make(map[string]int)}
			order = append(order, roomID)
		}
		if day.Valid {
			roomMap[roomID].Days[day.String] = count
		}
	}

	var items []RoomDayCount
	for _, id := range order {
		items = append(items, *roomMap[id])
	}
	return items, rows.Err()
}

func (a *Appointment) GetByID(id string) error {
	row := RDB.QueryRow(`SELECT `+appointmentColumns+` FROM appointments WHERE id = ?`, id)
	return a.ScanRow(row)
}

func (a *Appointment) Create() error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := checkAppointmentConflict(tx, a.RoomID, a.StartTime, a.EndTime, ""); err != nil {
		return err
	}

	a.ID = uuid.Must(uuid.NewV7()).String()
	now := DateNow()
	a.CreatedAt = now
	a.UpdatedAt = now

	_, err = tx.Exec(`INSERT INTO appointments (`+appointmentColumns+`)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		a.ID, a.PatientID, a.RoomID,
		a.StartTime, a.EndTime, a.Status,
		a.Notes, a.CreatedAt, a.UpdatedAt)
	if err != nil {
		return err
	}

	// If a procedure is specified, create the patient_procedure and link the appointment
	if a.ProcedureID != nil && *a.ProcedureID != "" {
		hasSessions := a.ProcedureSessionID != nil && *a.ProcedureSessionID != ""
		pp := PatientProcedure{
			PatientID:   a.PatientID,
			ProcedureID: *a.ProcedureID,
		}
		if !hasSessions {
			pp.AppointmentID = &a.ID
		}
		if err := pp.create(tx); err != nil {
			return err
		}
		if hasSessions {
			if _, err := tx.Exec(`UPDATE patient_procedure_sessions SET appointment_id = ?
				WHERE patient_procedure_id = ? AND procedure_session_id = ?`,
				a.ID, pp.ID, *a.ProcedureSessionID); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func (a *Appointment) Update(updates map[string]interface{}) error {
	cols := map[string]string{
		"patientId": "patient_id", "roomId": "room_id",
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
		if err := checkAppointmentConflict(DB, roomID, startTime, endTime, a.ID); err != nil {
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
