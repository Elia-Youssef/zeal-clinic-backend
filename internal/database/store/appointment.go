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
	RescheduledFrom string `json:"rescheduledFrom"`
	CreatedAt       Date   `json:"createdAt"`
	UpdatedAt       Date   `json:"updatedAt"`
	// Joined / transient: serves as input on create/update/reschedule
	// (procedureId, assignedToId, notes) and as the joined output.
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
	if msg := validation.OneOf(a.Status, []string{"Scheduled", "In-Progress", "Completed", "Cancelled", "Rescheduled"}, "Status"); msg != "" {
		e["status"] = msg
	}
	if len(e) > 0 {
		return e
	}
	return nil
}

const appointmentColumnsNoId = `patient_id, room_id, start_time, end_time, status, notes, cancel_notes, completion_notes, rescheduled_from, created_at, updated_at`
const appointmentColumns = `id, ` + appointmentColumnsNoId

const appointmentSelectQuery = `SELECT a.id, a.patient_id, a.room_id, a.start_time, a.end_time, a.status,
	a.notes, a.cancel_notes, a.completion_notes, COALESCE(a.rescheduled_from, ''), a.created_at, a.updated_at,
	COALESCE(p.first_name || ' ' || p.last_name, '') AS patient_name
	FROM appointments a
	LEFT JOIN patients p ON p.id = a.patient_id`

var appointmentSortColumns = map[string]string{
	"patientId":   "a.patient_id",
	"patientName": "patient_name",
	"roomId":      "a.room_id",
	"startTime":   "a.start_time",
	"endTime":     "a.end_time",
	"status":      "a.status",
	"createdAt":   "a.created_at",
	"updatedAt":   "a.updated_at",
}

type AppointmentList []Appointment

func (a *Appointment) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil appointment row")
	}
	return row.Scan(&a.ID, &a.PatientID, &a.RoomID,
		&a.StartTime, &a.EndTime, &a.Status,
		&a.Notes, &a.CancelNotes, &a.CompletionNotes,
		&a.RescheduledFrom, &a.CreatedAt, &a.UpdatedAt, &a.PatientName)
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
			&item.RescheduledFrom, &item.CreatedAt, &item.UpdatedAt, &item.PatientName)
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

	apRows, err := RDB.Query(`SELECT ap.id, ap.patient_id, ap.procedure_id, ap.appointment_id,
		COALESCE(ap.assigned_to_id, ''), ap.notes,
		ap.created_at, ap.updated_at, pr.name, COALESCE(c.name, ''),
		COALESCE(e.first_name || ' ' || e.last_name, '')
		FROM appointment_procedures ap
		JOIN procedures pr ON pr.id = ap.procedure_id
		LEFT JOIN procedure_categories c ON c.id = pr.category_id
		LEFT JOIN employees e ON e.id = ap.assigned_to_id
		WHERE ap.appointment_id IN (`+placeholders+`)
		ORDER BY ap.created_at`, ids...)
	if err != nil {
		return err
	}
	defer apRows.Close()
	for apRows.Next() {
		var ap AppointmentProcedure
		var catName string
		if err := apRows.Scan(&ap.ID, &ap.PatientID, &ap.ProcedureID, &ap.AppointmentID,
			&ap.AssignedToID, &ap.Notes,
			&ap.CreatedAt, &ap.UpdatedAt, &ap.ProcedureName, &catName, &ap.AssignedToName); err != nil {
			continue
		}
		if catName != "" {
			ap.ProcedureName = catName + ", " + ap.ProcedureName
		}
		if idx, ok := idIdx[ap.AppointmentID]; ok {
			(*l)[idx].AppointmentProcedures = append((*l)[idx].AppointmentProcedures, ap)
		}
	}

	return nil
}

// nullableID returns nil for an empty id so the value is stored as SQL NULL,
// which lets foreign-key constraints accept "no parent".
func nullableID(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func checkAppointmentConflict(db DBTX, roomID string, startTime, endTime Date, excludeID string) error {
	query := `SELECT COUNT(*) FROM appointments
		WHERE room_id = ?
		AND status NOT IN ('Cancelled','Rescheduled')
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
	// `date` is a clinic-local calendar day (YYYY-MM-DD) picked by the user.
	// Convert to UTC half-open instants so a 1 AM Beirut appointment (which
	// is stored as the previous UTC day) is still attributed to the right day.
	t, err := time.Parse(DateFormat, date)
	if err != nil {
		t = ClinicNow()
	}
	dayStart, dayEnd := ClinicDayBounds(t)
	return a.getAllBetween(dayStart, dayEnd, params)
}

// GetWeek loads appointments for the Monday-Sunday week containing date,
// matching the weekly appointment grid (GetAppointmentCountPerRoom).
func (a *AppointmentList) GetWeek(date string, params ListParams) (int, error) {
	dayStart, dayEnd := weekBounds(date)
	return a.getAllBetween(dayStart, dayEnd, params)
}

// weekBounds returns the half-open UTC instants of the Monday-Sunday clinic
// week containing date. Empty/invalid date falls back to the current week.
func weekBounds(date string) (Date, Date) {
	t, err := time.Parse(DateFormat, date)
	if err != nil {
		t = ClinicNow()
	}
	offset := int(t.Weekday() - time.Monday)
	if offset < 0 {
		offset = 6
	}
	weekStart := t.AddDate(0, 0, -offset)
	dayStart, _ := ClinicDayBounds(weekStart)
	_, dayEnd := ClinicDayBounds(weekStart.AddDate(0, 0, 6))
	return dayStart, dayEnd
}

// getAllBetween loads appointments whose start_time falls in the half-open
// UTC range [dayStart, dayEnd), excluding cancelled/rescheduled rows.
func (a *AppointmentList) getAllBetween(dayStart, dayEnd Date, params ListParams) (int, error) {
	return a.listByWhere(
		" WHERE a.start_time >= ? AND a.start_time < ? AND a.status NOT IN ('Cancelled','Rescheduled')",
		[]any{dayStart, dayEnd},
		"a.start_time, (SELECT name FROM rooms WHERE id = a.room_id)",
		params,
	)
}

type RoomDayCount struct {
	RoomID   string         `json:"roomId"`
	RoomName string         `json:"roomName"`
	Days     map[string]int `json:"days"`
}

func GetAppointmentCountPerRoom(weekStart, weekEnd Date) ([]RoomDayCount, error) {
	query := `SELECT r.id, r.name, DATE(a.start_time) as day, COUNT(a.id) as count
		FROM rooms r
		LEFT JOIN appointments a ON a.room_id = r.id
			AND a.start_time >= ? AND a.start_time < ?
			AND a.status NOT IN ('Cancelled','Rescheduled')
		GROUP BY r.id, r.name, DATE(a.start_time)
		ORDER BY r.name, day`
	rows, err := RDB.Query(query, RangeStart(string(weekStart)), RangeEnd(string(weekEnd)))
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

// listByWhere is the shared body of every appointment list query: it appends
// the standard status/notes filter, counts matching rows, runs the select with
// the caller's sort, and hydrates each appointment's procedures.
func (a *AppointmentList) listByWhere(where string, args []any, defaultSort string, params ListParams) (int, error) {
	if fc, fa := params.FilterClause("a.status", "a.notes"); fc != "" {
		where += " AND " + fc
		args = append(args, fa...)
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*) FROM appointments a"+where, args...).Scan(&total); err != nil {
		return 0, err
	}

	order := params.OrderClause(appointmentSortColumns, defaultSort)
	rows, err := RDB.Query(appointmentSelectQuery+where+order+params.PaginationClause(), args...)
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
	return total, a.LoadProcedures()
}

func (a *AppointmentList) GetByPatientID(patientID string, params ListParams) (int, error) {
	return a.listByWhere(" WHERE a.patient_id = ?", []any{patientID}, "a.start_time DESC", params)
}

// GetByEmployeeWeek loads the employee's appointments for the Monday-Sunday
// week containing date. Empty/invalid date falls back to the current week.
func (a *AppointmentList) GetByEmployeeWeek(employeeID string, date string, params ListParams) (int, error) {
	dayStart, dayEnd := weekBounds(date)
	return a.listByWhere(
		" WHERE a.start_time >= ? AND a.start_time < ? AND a.id IN (SELECT appointment_id FROM appointment_procedures WHERE assigned_to_id = ?)",
		[]any{dayStart, dayEnd, employeeID}, "a.start_time", params)
}

func (a *AppointmentList) GetByProcedureID(procedureID string, params ListParams) (int, error) {
	return a.listByWhere(
		" WHERE a.id IN (SELECT appointment_id FROM appointment_procedures WHERE procedure_id = ?)",
		[]any{procedureID}, "a.start_time DESC", params)
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

	if _, err := tx.Exec(`INSERT INTO appointments (`+appointmentColumns+`)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		a.ID, a.PatientID, a.RoomID,
		a.StartTime, a.EndTime, a.Status,
		a.Notes, a.CancelNotes, a.CompletionNotes,
		nullableID(a.RescheduledFrom), a.CreatedAt, a.UpdatedAt); err != nil {
		return err
	}

	if err := syncAppointmentProcedures(tx, a.ID, a.PatientID, a.AppointmentProcedures); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	RDB.QueryRow(`SELECT COALESCE(first_name || ' ' || last_name, '') FROM patients WHERE id = ?`, a.PatientID).Scan(&a.PatientName)
	return nil
}

func (a *Appointment) Update(updates map[string]any) error {
	procedures, hasProcedures := parseProceduresUpdate(updates)

	cols := map[string]string{
		"patientId": "patient_id", "roomId": "room_id",
		"startTime": "start_time", "endTime": "end_time",
		"status": "status", "notes": "notes",
		"cancelNotes": "cancel_notes", "completionNotes": "completion_notes",
	}
	setClauses := ""
	var args []any
	for jsonKey, dbCol := range cols {
		val, ok := updates[jsonKey]
		if !ok {
			continue
		}
		if setClauses != "" {
			setClauses += ", "
		}
		setClauses += dbCol + " = ?"
		args = append(args, val)
	}
	if setClauses == "" && !hasProcedures {
		return a.GetByID(a.ID)
	}

	var current Appointment
	_, roomChanged := updates["roomId"]
	_, startChanged := updates["startTime"]
	_, endChanged := updates["endTime"]
	if roomChanged || startChanged || endChanged || hasProcedures {
		if err := current.GetByID(a.ID); err != nil {
			return err
		}
	}

	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

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
		if err := checkAppointmentConflict(tx, roomID, startTime, endTime, a.ID); err != nil {
			return err
		}
	}

	if setClauses != "" {
		setClauses += ", updated_at = ?"
		args = append(args, DateNow(), a.ID)
		if _, err := tx.Exec("UPDATE appointments SET "+setClauses+" WHERE id = ?", args...); err != nil {
			return err
		}
	}

	if hasProcedures {
		patientID := current.PatientID
		if v, ok := updates["patientId"].(string); ok {
			patientID = v
		}
		if err := syncAppointmentProcedures(tx, a.ID, patientID, procedures); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return a.GetByID(a.ID)
}

func (a *Appointment) Reschedule(overrides map[string]any) error {
	var old Appointment
	old.ID = a.ID
	if err := old.GetByID(old.ID); err != nil {
		return err
	}
	if old.Status == "Cancelled" {
		return errors.New("cannot reschedule a cancelled appointment")
	}
	if old.Status == "Rescheduled" {
		return errors.New("cannot reschedule an already-rescheduled appointment")
	}

	// Hydrate the old procedures so they (and any nurse assignments) carry over
	// unless overrides explicitly replace them.
	oldList := AppointmentList{old}
	if err := oldList.LoadProcedures(); err != nil {
		return err
	}
	old = oldList[0]

	newApt := Appointment{
		PatientID:             old.PatientID,
		RoomID:                old.RoomID,
		StartTime:             old.StartTime,
		EndTime:               old.EndTime,
		Status:                "Scheduled",
		Notes:                 old.Notes,
		AppointmentProcedures: old.AppointmentProcedures,
	}
	if v, ok := overrides["patientId"].(string); ok && v != "" {
		newApt.PatientID = v
	}
	if v, ok := overrides["roomId"].(string); ok && v != "" {
		newApt.RoomID = v
	}
	if v, ok := overrides["startTime"].(string); ok && v != "" {
		newApt.StartTime = Date(v)
	}
	if v, ok := overrides["endTime"].(string); ok && v != "" {
		newApt.EndTime = Date(v)
	}
	if v, ok := overrides["status"].(string); ok && v != "" {
		newApt.Status = v
	}
	if v, ok := overrides["notes"].(string); ok {
		newApt.Notes = v
	}
	if procedures, ok := parseProceduresUpdate(overrides); ok {
		newApt.AppointmentProcedures = procedures
	}

	if err := newApt.IsValid(); err != nil {
		return err
	}

	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	newApt.ID = uuid.Must(uuid.NewV7()).String()
	newApt.RescheduledFrom = old.ID
	now := DateNow()
	newApt.CreatedAt = now
	newApt.UpdatedAt = now

	// Mark the old appointment Rescheduled first so it does not block the new
	// one in checkAppointmentConflict (which ignores Cancelled/Rescheduled rows).
	cancelNotes := old.CancelNotes
	if v, ok := overrides["cancelNotes"].(string); ok {
		cancelNotes = v
	}
	if _, err := tx.Exec(
		`UPDATE appointments SET status='Rescheduled', cancel_notes = ?, updated_at = ? WHERE id = ?`,
		cancelNotes, now, old.ID,
	); err != nil {
		return err
	}

	if err := checkAppointmentConflict(tx, newApt.RoomID, newApt.StartTime, newApt.EndTime, ""); err != nil {
		return err
	}

	if _, err := tx.Exec(`INSERT INTO appointments (`+appointmentColumns+`)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		newApt.ID, newApt.PatientID, newApt.RoomID,
		newApt.StartTime, newApt.EndTime, newApt.Status,
		newApt.Notes, newApt.CancelNotes, newApt.CompletionNotes,
		nullableID(newApt.RescheduledFrom), newApt.CreatedAt, newApt.UpdatedAt,
	); err != nil {
		return err
	}

	if err := syncAppointmentProcedures(tx, newApt.ID, newApt.PatientID, newApt.AppointmentProcedures); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	*a = Appointment{ID: newApt.ID}
	return a.GetByID(a.ID)
}

func (a *Appointment) Delete() error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM appointment_procedures WHERE appointment_id = ?`, a.ID); err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM appointments WHERE id = ?`, a.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

// parseProceduresUpdate reads "appointmentProcedures" from a JSON-bound updates
// map and returns the parsed list plus a flag indicating whether the field was
// present (so callers can tell "not provided" apart from "provided, empty").
func parseProceduresUpdate(updates map[string]any) ([]AppointmentProcedure, bool) {
	v, ok := updates["appointmentProcedures"]
	if !ok {
		return nil, false
	}
	arr, _ := v.([]any)
	out := make([]AppointmentProcedure, 0, len(arr))
	for _, item := range arr {
		m, _ := item.(map[string]any)
		procID, _ := m["procedureId"].(string)
		if procID == "" {
			continue
		}
		assignedTo, _ := m["assignedToId"].(string)
		notes, _ := m["notes"].(string)
		out = append(out, AppointmentProcedure{
			ProcedureID:  procID,
			AssignedToID: assignedTo,
			Notes:        notes,
		})
	}
	return out, true
}

// syncAppointmentProcedures reconciles appointment_procedures linked to an
// appointment so they match the supplied list: rows whose procedure is no
// longer present are deleted, new procedures are inserted, and existing rows
// have their assigned_to_id / notes updated when they differ.
func syncAppointmentProcedures(tx DBTX, appointmentID, patientID string, inputs []AppointmentProcedure) error {
	type row struct{ id, assignedToID, notes string }
	existing := map[string]row{}
	rows, err := tx.Query(`SELECT id, procedure_id, COALESCE(assigned_to_id, ''), notes
		FROM appointment_procedures WHERE appointment_id = ?`, appointmentID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var r row
		var procID string
		if err := rows.Scan(&r.id, &procID, &r.assignedToID, &r.notes); err != nil {
			rows.Close()
			return err
		}
		existing[procID] = r
	}
	rows.Close()

	wanted := make(map[string]bool, len(inputs))
	for _, in := range inputs {
		if in.ProcedureID != "" {
			wanted[in.ProcedureID] = true
		}
	}
	for procID, r := range existing {
		if wanted[procID] {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM appointment_procedures WHERE id = ?`, r.id); err != nil {
			return err
		}
	}

	for _, in := range inputs {
		if in.ProcedureID == "" {
			continue
		}
		r, exists := existing[in.ProcedureID]
		if !exists {
			ap := AppointmentProcedure{
				PatientID:     patientID,
				ProcedureID:   in.ProcedureID,
				AppointmentID: appointmentID,
				AssignedToID:  in.AssignedToID,
				Notes:         in.Notes,
			}
			if err := ap.create(tx); err != nil {
				return err
			}
			continue
		}
		if r.assignedToID == in.AssignedToID && r.notes == in.Notes {
			continue
		}
		if _, err := tx.Exec(`UPDATE appointment_procedures
			SET assigned_to_id = ?, notes = ?, updated_at = ? WHERE id = ?`,
			nullableID(in.AssignedToID), in.Notes, DateNow(), r.id); err != nil {
			return err
		}
	}
	return nil
}
