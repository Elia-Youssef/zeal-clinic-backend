package store

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

// One-off modification to an employee's schedule template. 'overtime' adds the
// (start_time, end_time) window to every day in range; 'timeoff' removes it (or
// the whole day when times are empty). Only 'accepted' rows affect projection.
type EmployeeScheduleChange struct {
	ID           string `json:"id"`
	EmployeeID   string `json:"employeeId"`
	Type         string `json:"type"`
	StartDate    Date   `json:"startDate"`
	EndDate      Date   `json:"endDate"`
	StartTime    string `json:"startTime,omitempty"`
	EndTime      string `json:"endTime,omitempty"`
	Status       string `json:"status"`
	Notes        string `json:"notes"`
	CreatedAt    Date   `json:"createdAt"`
	UpdatedAt    Date   `json:"updatedAt"`
	EmployeeName string `json:"employeeName,omitempty"`
}

var EmployeeScheduleChangeStatuses = []string{"pending", "accepted", "rejected"}
var EmployeeScheduleChangeTypes = []string{"timeoff", "overtime"}

const employeeScheduleChangeColumnsNoId = `employee_id, type, start_date, end_date, start_time, end_time, status, notes, created_at, updated_at`
const employeeScheduleChangeColumns = `id, ` + employeeScheduleChangeColumnsNoId

type EmployeeScheduleChangeList []EmployeeScheduleChange

func (v *EmployeeScheduleChange) IsValid() error {
	e := make(validation.Errors)
	if msg := validation.Required(v.EmployeeID, "Employee ID"); msg != "" {
		e["employeeId"] = msg
	}
	if msg := validation.OneOf(v.Type, EmployeeScheduleChangeTypes, "Type"); msg != "" {
		e["type"] = msg
	}
	if v.StartDate.IsZero() {
		e["startDate"] = "Start date is required"
	}
	if v.EndDate.IsZero() {
		e["endDate"] = "End date is required"
	}
	if !v.StartDate.IsZero() && !v.EndDate.IsZero() && v.EndDate.Before(v.StartDate) {
		e["endDate"] = "End date must be on or after start date"
	}
	if (v.StartTime != "") != (v.EndTime != "") {
		e["time"] = "Start time and end time must both be set or both empty"
	}
	if v.Type == "overtime" && v.StartTime == "" {
		e["time"] = "Overtime requires start time and end time"
	}
	if v.StartTime != "" && v.EndTime != "" {
		startMin, ok1 := minutesOfDay(v.StartTime)
		endMin, ok2 := minutesOfDay(v.EndTime)
		if !ok1 || !ok2 {
			e["time"] = "Invalid start or end time"
		} else if endMin <= startMin {
			e["time"] = "End time must be after start time"
		}
	}
	if v.Status != "" {
		if msg := validation.OneOf(v.Status, EmployeeScheduleChangeStatuses, "Status"); msg != "" {
			e["status"] = msg
		}
	}
	if len(e) > 0 {
		return e
	}
	return nil
}

func employeeScheduleChangeSelect() string {
	return `SELECT sc.id, sc.employee_id, sc.type, sc.start_date, sc.end_date, sc.start_time, sc.end_time,
		sc.status, sc.notes, sc.created_at, sc.updated_at,
		e.first_name || ' ' || e.last_name`
}

func (v *EmployeeScheduleChange) scanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil EmployeeScheduleChange row")
	}
	return row.Scan(&v.ID, &v.EmployeeID, &v.Type, &v.StartDate, &v.EndDate, &v.StartTime, &v.EndTime,
		&v.Status, &v.Notes, &v.CreatedAt, &v.UpdatedAt, &v.EmployeeName)
}

func (l *EmployeeScheduleChangeList) scanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil EmployeeScheduleChange rows")
	}
	*l = EmployeeScheduleChangeList{}
	for rows.Next() {
		var item EmployeeScheduleChange
		if err := rows.Scan(&item.ID, &item.EmployeeID, &item.Type, &item.StartDate, &item.EndDate, &item.StartTime, &item.EndTime,
			&item.Status, &item.Notes, &item.CreatedAt, &item.UpdatedAt, &item.EmployeeName); err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (v *EmployeeScheduleChange) GetByID(id string) error {
	return v.scanRow(RDB.QueryRow(employeeScheduleChangeSelect()+`
		FROM employee_schedule_changes sc
		JOIN employees e ON e.id = sc.employee_id
		WHERE sc.id = ?`, id))
}

// EmployeeScheduleChangesOverlappingRange returns changes (any status) that
// intersect [rangeStart, rangeEnd]. employeeID == "" returns every employee's.
func EmployeeScheduleChangesOverlappingRange(employeeID string, rangeStart, rangeEnd Date) (EmployeeScheduleChangeList, error) {
	where := ` WHERE sc.end_date >= ? AND sc.start_date <= ?`
	args := []any{rangeStart, rangeEnd}
	if employeeID != "" {
		where += ` AND sc.employee_id = ?`
		args = append(args, employeeID)
	}
	rows, err := RDB.Query(employeeScheduleChangeSelect()+`
		FROM employee_schedule_changes sc
		JOIN employees e ON e.id = sc.employee_id`+where+`
		ORDER BY sc.start_date, e.first_name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items EmployeeScheduleChangeList
	if err := items.scanRows(rows); err != nil {
		return nil, err
	}
	return items, rows.Err()
}

func (v *EmployeeScheduleChange) Create() error {
	if v.Status == "" {
		v.Status = "pending"
	}
	v.StartDate = Date(v.StartDate.DateOnly())
	v.EndDate = Date(v.EndDate.DateOnly())
	if err := v.IsValid(); err != nil {
		return err
	}
	now := DateNow()
	v.ID = uuid.Must(uuid.NewV7()).String()
	v.CreatedAt = now
	v.UpdatedAt = now
	_, err := DB.Exec(`INSERT INTO employee_schedule_changes (`+employeeScheduleChangeColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		v.ID, v.EmployeeID, v.Type, v.StartDate, v.EndDate, v.StartTime, v.EndTime, v.Status, v.Notes, v.CreatedAt, v.UpdatedAt)
	if err != nil {
		return err
	}
	return v.GetByID(v.ID)
}

func (v *EmployeeScheduleChange) Update(updates map[string]any) error {
	var current EmployeeScheduleChange
	if err := current.GetByID(v.ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	next := current
	if val, ok := stringUpdate(updates, "type"); ok {
		next.Type = val
	}
	if val, ok := dateUpdate(updates, "startDate"); ok {
		next.StartDate = val
	}
	if val, ok := dateUpdate(updates, "endDate"); ok {
		next.EndDate = val
	}
	if val, ok := stringUpdate(updates, "startTime"); ok {
		next.StartTime = val
	}
	if val, ok := stringUpdate(updates, "endTime"); ok {
		next.EndTime = val
	}
	if val, ok := stringUpdate(updates, "notes"); ok {
		next.Notes = val
	}
	next.StartDate = Date(next.StartDate.DateOnly())
	next.EndDate = Date(next.EndDate.DateOnly())
	if err := next.IsValid(); err != nil {
		return err
	}
	now := DateNow()
	if _, err := DB.Exec(`UPDATE employee_schedule_changes SET type = ?, start_date = ?, end_date = ?, start_time = ?, end_time = ?, notes = ?, updated_at = ? WHERE id = ?`,
		next.Type, next.StartDate, next.EndDate, next.StartTime, next.EndTime, next.Notes, now, next.ID); err != nil {
		return err
	}
	next.UpdatedAt = now
	*v = next
	return nil
}

func (v *EmployeeScheduleChange) Delete() error {
	res, err := DB.Exec(`DELETE FROM employee_schedule_changes WHERE id = ?`, v.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (v *EmployeeScheduleChange) SetStatus(status string) error {
	if msg := validation.OneOf(status, EmployeeScheduleChangeStatuses, "Status"); msg != "" {
		return validation.Errors{"status": msg}
	}
	now := DateNow()
	res, err := DB.Exec(`UPDATE employee_schedule_changes SET status = ?, updated_at = ? WHERE id = ?`, status, now, v.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return v.GetByID(v.ID)
}
