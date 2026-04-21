package store

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Appointment struct {
	ID              string `json:"id"`
	PatientID       string `json:"patientId"`
	RoomID          string `json:"roomId"`
	StartTime       Date   `json:"startTime"`
	EndTime         Date   `json:"endTime"`
	Status          string `json:"status"`
	Notes           string `json:"notes"`
	CancelNotes     string `json:"cancelNotes"`
	CompletionNotes string `json:"completionNotes"`
	CreatedAt       Date   `json:"createdAt"`
	UpdatedAt       Date   `json:"updatedAt"`
	// Transient fields (not stored in appointments table)
	ProcedureID        *string `json:"procedureId,omitempty"`
	ProcedureSessionID *string `json:"procedureSessionId,omitempty"`
	// Joined fields
	PatientName             string                   `json:"patientName,omitempty"`
	PatientProcedure        *PatientProcedure        `json:"patientProcedure,omitempty"`
	PatientProcedureSession *PatientProcedureSession `json:"patientProcedureSession,omitempty"`
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

const appointmentColumnsNoId = `patient_id, room_id, start_time, end_time, status, notes, cancel_notes, completion_notes, created_at, updated_at`
const appointmentColumns = `id, ` + appointmentColumnsNoId

const appointmentSelectQuery = `SELECT a.id, a.patient_id, a.room_id, a.start_time, a.end_time, a.status,
	a.notes, a.cancel_notes, a.completion_notes, a.created_at, a.updated_at,
	COALESCE(p.first_name || ' ' || p.last_name, '') AS patient_name
	FROM appointments a
	LEFT JOIN patients p ON p.id = a.patient_id`

type AppointmentList []Appointment

func (a *Appointment) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil appointment row")
	}
	err := row.Scan(&a.ID, &a.PatientID, &a.RoomID,
		&a.StartTime, &a.EndTime, &a.Status,
		&a.Notes, &a.CancelNotes, &a.CompletionNotes,
		&a.CreatedAt, &a.UpdatedAt, &a.PatientName)
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
			&item.Notes, &item.CancelNotes, &item.CompletionNotes,
			&item.CreatedAt, &item.UpdatedAt, &item.PatientName)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

// LoadProcedures populates PatientProcedure and PatientProcedureSession for each appointment.
func (l *AppointmentList) LoadProcedures() error {
	if len(*l) == 0 {
		return nil
	}

	ids := make([]any, len(*l))
	idIdx := make(map[string]int, len(*l))
	placeholders := ""
	for i, a := range *l {
		ids[i] = a.ID
		idIdx[a.ID] = i
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
	}

	// Patient procedures linked directly to appointments
	ppRows, err := RDB.Query(`SELECT pp.id, pp.patient_id, pp.procedure_id, pp.appointment_id, pp.status, pp.notes,
		pp.created_at, pp.updated_at, pr.name
		FROM patient_procedures pp
		JOIN procedures pr ON pr.id = pp.procedure_id
		WHERE pp.appointment_id IN (`+placeholders+`)`, ids...)
	if err != nil {
		return err
	}
	defer ppRows.Close()
	for ppRows.Next() {
		var pp PatientProcedure
		if err := ppRows.Scan(&pp.ID, &pp.PatientID, &pp.ProcedureID, &pp.AppointmentID, &pp.Status, &pp.Notes,
			&pp.CreatedAt, &pp.UpdatedAt, &pp.ProcedureName); err != nil {
			continue
		}
		if idx, ok := idIdx[*pp.AppointmentID]; ok {
			(*l)[idx].PatientProcedure = &pp
		}
	}

	// Patient procedure sessions linked to appointments (with their parent procedure)
	ppsRows, err := RDB.Query(`SELECT pps.id, pps.patient_procedure_id, pps.procedure_session_id, pps.appointment_id,
		pps.status, pps.notes, pps.created_at, pps.updated_at,
		pp.id, pp.patient_id, pp.procedure_id, pp.appointment_id, pp.status, pp.notes,
		pp.created_at, pp.updated_at, pr.name
		FROM patient_procedure_sessions pps
		JOIN patient_procedures pp ON pp.id = pps.patient_procedure_id
		JOIN procedures pr ON pr.id = pp.procedure_id
		WHERE pps.appointment_id IN (`+placeholders+`)`, ids...)
	if err != nil {
		return err
	}
	defer ppsRows.Close()
	for ppsRows.Next() {
		var pps PatientProcedureSession
		var pp PatientProcedure
		if err := ppsRows.Scan(&pps.ID, &pps.PatientProcedureID, &pps.ProcedureSessionID,
			&pps.AppointmentID, &pps.Status, &pps.Notes, &pps.CreatedAt, &pps.UpdatedAt,
			&pp.ID, &pp.PatientID, &pp.ProcedureID, &pp.AppointmentID, &pp.Status, &pp.Notes,
			&pp.CreatedAt, &pp.UpdatedAt, &pp.ProcedureName); err != nil {
			continue
		}
		if idx, ok := idIdx[*pps.AppointmentID]; ok {
			(*l)[idx].PatientProcedureSession = &pps
			if (*l)[idx].PatientProcedure == nil {
				(*l)[idx].PatientProcedure = &pp
			}
		}
	}

	return nil
}

func checkAppointmentConflict(db DBTX, roomID string, startTime, endTime Date, excludeID string) error {
	query := `SELECT COUNT(*) FROM appointments
		WHERE room_id = ?
		AND status != 'Cancelled'
		AND start_time < ?
		AND end_time > ?`
	args := []any{roomID, endTime, startTime}
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
	where := " WHERE DATE(a.start_time) = ? AND a.status != 'Cancelled'"
	args := []any{date}
	if fc, fa := params.FilterClause("a.status", "a.notes"); fc != "" {
		where += " AND " + fc
		args = append(args, fa...)
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*) FROM appointments a"+where, args...).Scan(&total); err != nil {
		return 0, err
	}

	query := appointmentSelectQuery + where + ` ORDER BY a.start_time` + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	if err := a.ScanRows(rows); err != nil {
		return 0, err
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if err := a.LoadProcedures(); err != nil {
		return 0, err
	}
	return total, nil
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

func (a *AppointmentList) GetByPatientID(patientID string, params ListParams) (int, error) {
	where := " WHERE a.patient_id = ?"
	args := []any{patientID}
	if fc, fa := params.FilterClause("a.status", "a.notes"); fc != "" {
		where += " AND " + fc
		args = append(args, fa...)
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*) FROM appointments a"+where, args...).Scan(&total); err != nil {
		return 0, err
	}

	query := appointmentSelectQuery + where + ` ORDER BY a.start_time DESC` + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	if err := a.ScanRows(rows); err != nil {
		return 0, err
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if err := a.LoadProcedures(); err != nil {
		return 0, err
	}
	return total, nil
}

func (a *Appointment) GetByID(id string) error {
	row := RDB.QueryRow(appointmentSelectQuery+` WHERE a.id = ?`, id)
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
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		a.ID, a.PatientID, a.RoomID,
		a.StartTime, a.EndTime, a.Status,
		a.Notes, a.CancelNotes, a.CompletionNotes,
		a.CreatedAt, a.UpdatedAt)
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

	if err := tx.Commit(); err != nil {
		return err
	}

	RDB.QueryRow(`SELECT COALESCE(first_name || ' ' || last_name, '') FROM patients WHERE id = ?`, a.PatientID).Scan(&a.PatientName)
	return nil
}

func (a *Appointment) Update(updates map[string]any) error {
	cols := map[string]string{
		"patientId": "patient_id", "roomId": "room_id",
		"startTime": "start_time", "endTime": "end_time",
		"status": "status", "notes": "notes",
		"cancelNotes": "cancel_notes", "completionNotes": "completion_notes",
	}

	setClauses := ""
	var args []any
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
		return ErrNotFound
	}
	return nil
}
