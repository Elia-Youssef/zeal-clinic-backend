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
	ProcedureIDs []string `json:"procedureIds,omitempty"`
	// Joined fields
	PatientName           string                   `json:"patientName,omitempty"`
	AppointmentProcedures AppointmentProcedureList `json:"appointmentProcedures,omitempty"`
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

// LoadProcedures populates AppointmentProcedures for each appointment.
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
		(*l)[i].AppointmentProcedures = AppointmentProcedureList{}
	}

	apRows, err := RDB.Query(`SELECT ap.id, ap.patient_id, ap.procedure_id, ap.appointment_id, ap.notes,
		ap.created_at, ap.updated_at, pr.name, COALESCE(c.name, ''), COALESCE(pc.name, '')
		FROM appointment_procedures ap
		JOIN procedures pr ON pr.id = ap.procedure_id
		LEFT JOIN procedure_categories c ON c.id = pr.category_id
		LEFT JOIN procedure_categories pc ON pc.id = c.parent_id
		WHERE ap.appointment_id IN (`+placeholders+`)
		ORDER BY ap.created_at`, ids...)
	if err != nil {
		return err
	}
	defer apRows.Close()
	for apRows.Next() {
		var ap AppointmentProcedure
		var catName, parentName string
		if err := apRows.Scan(&ap.ID, &ap.PatientID, &ap.ProcedureID, &ap.AppointmentID, &ap.Notes,
			&ap.CreatedAt, &ap.UpdatedAt, &ap.ProcedureName, &catName, &parentName); err != nil {
			continue
		}
		if parentName != "" {
			ap.ProcedureName = parentName + ", " + catName + ", " + ap.ProcedureName
		} else if catName != "" {
			ap.ProcedureName = catName + ", " + ap.ProcedureName
		}
		if idx, ok := idIdx[ap.AppointmentID]; ok {
			(*l)[idx].AppointmentProcedures = append((*l)[idx].AppointmentProcedures, ap)
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

func (a *AppointmentList) GetByProcedureID(procedureID string, params ListParams) (int, error) {
	where := ` WHERE a.id IN (SELECT appointment_id FROM appointment_procedures WHERE procedure_id = ?)`
	args := []any{procedureID}
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

	// Create an appointment_procedure for each linked procedure.
	for _, pid := range a.ProcedureIDs {
		if pid == "" {
			continue
		}
		ap := AppointmentProcedure{
			PatientID:     a.PatientID,
			ProcedureID:   pid,
			AppointmentID: a.ID,
		}
		if err := ap.create(tx); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	RDB.QueryRow(`SELECT COALESCE(first_name || ' ' || last_name, '') FROM patients WHERE id = ?`, a.PatientID).Scan(&a.PatientName)
	return nil
}

func (a *Appointment) Update(updates map[string]any) error {
	// procedureIds is a transient field, not a column. Pull it out and apply
	// it as a sync against appointment_procedures inside the same transaction.
	var procedureIDs []string
	hasProcedureIDs := false
	if v, ok := updates["procedureIds"]; ok {
		hasProcedureIDs = true
		delete(updates, "procedureIds")
		if arr, ok := v.([]any); ok {
			for _, item := range arr {
				if s, ok := item.(string); ok && s != "" {
					procedureIDs = append(procedureIDs, s)
				}
			}
		}
	}

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
	if setClauses == "" && !hasProcedureIDs {
		return a.GetByID(a.ID)
	}

	var current Appointment
	_, roomChanged := updates["roomId"]
	_, startChanged := updates["startTime"]
	_, endChanged := updates["endTime"]
	if roomChanged || startChanged || endChanged || hasProcedureIDs {
		if err := current.GetByID(a.ID); err != nil {
			return err
		}
	}
	if roomChanged || startChanged || endChanged {
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

	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if setClauses != "" {
		setClauses += ", updated_at = ?"
		args = append(args, DateNow())
		args = append(args, a.ID)
		if _, err := tx.Exec("UPDATE appointments SET "+setClauses+" WHERE id = ?", args...); err != nil {
			return err
		}
	}

	if hasProcedureIDs {
		patientID := current.PatientID
		if v, ok := updates["patientId"].(string); ok {
			patientID = v
		}
		if err := syncAppointmentProcedures(tx, a.ID, patientID, procedureIDs); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return a.GetByID(a.ID)
}

// syncAppointmentProcedures reconciles the appointment_procedures linked to
// an appointment so they match the supplied procedure id set: rows whose
// procedure is no longer in the list are deleted, and rows for newly added
// procedures are inserted.
func syncAppointmentProcedures(tx DBTX, appointmentID, patientID string, procedureIDs []string) error {
	rows, err := tx.Query(`SELECT id, procedure_id FROM appointment_procedures WHERE appointment_id = ?`, appointmentID)
	if err != nil {
		return err
	}
	existingByProc := make(map[string]string)
	for rows.Next() {
		var apID, procID string
		if err := rows.Scan(&apID, &procID); err != nil {
			rows.Close()
			return err
		}
		existingByProc[procID] = apID
	}
	rows.Close()

	newSet := make(map[string]bool, len(procedureIDs))
	for _, pid := range procedureIDs {
		newSet[pid] = true
	}

	for procID, apID := range existingByProc {
		if !newSet[procID] {
			if _, err := tx.Exec(`DELETE FROM appointment_procedures WHERE id = ?`, apID); err != nil {
				return err
			}
		}
	}

	for _, procID := range procedureIDs {
		if _, exists := existingByProc[procID]; exists {
			continue
		}
		ap := AppointmentProcedure{
			PatientID:     patientID,
			ProcedureID:   procID,
			AppointmentID: appointmentID,
		}
		if err := ap.create(tx); err != nil {
			return err
		}
	}
	return nil
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
