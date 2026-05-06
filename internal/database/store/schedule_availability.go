package store

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

const scheduleAvailabilityColumnsNoId = `employee_id, day_of_week, start_time, end_time, start_date, end_date, is_active, created_at, updated_at`
const scheduleAvailabilityColumns = `id, ` + scheduleAvailabilityColumnsNoId

type ScheduleAvailability struct {
	ID         string `json:"id"`
	EmployeeID string `json:"employeeId"`
	DayOfWeek  int    `json:"dayOfWeek"`
	StartTime  string `json:"startTime"`
	EndTime    string `json:"endTime"`
	StartDate  Date   `json:"startDate"`
	EndDate    Date   `json:"endDate,omitempty"`
	IsActive   bool   `json:"isActive"`
	CreatedAt  Date   `json:"createdAt"`
	UpdatedAt  Date   `json:"updatedAt"`
	// Joined fields
	EmployeeName string `json:"employeeName,omitempty"`
}

func (sa *ScheduleAvailability) IsValid() error {
	e := make(validation.Errors)
	if msg := validation.Required(sa.EmployeeID, "Employee ID"); msg != "" {
		e["employeeId"] = msg
	}
	if msg := validation.Required(sa.StartTime, "Start time"); msg != "" {
		e["startTime"] = msg
	}
	if msg := validation.Required(sa.EndTime, "End time"); msg != "" {
		e["endTime"] = msg
	}
	if sa.DayOfWeek < 0 || sa.DayOfWeek > 6 {
		e["dayOfWeek"] = "Day of week must be between 0 and 6"
	}
	if len(e) > 0 {
		return e
	}
	return nil
}

func (sa *ScheduleAvailability) normalizeDates() {
	if sa.StartDate.IsZero() {
		sa.StartDate = DateToday()
	}
}

type ScheduleAvailabilityList []ScheduleAvailability

func (m *ScheduleAvailability) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil ScheduleAvailability row")
	}
	var isActive int
	err := row.Scan(&m.ID, &m.EmployeeID, &m.DayOfWeek, &m.StartTime, &m.EndTime,
		&m.StartDate, &m.EndDate, &isActive, &m.CreatedAt, &m.UpdatedAt, &m.EmployeeName)
	if err != nil {
		return err
	}
	m.IsActive = isActive == 1
	return nil
}

func (l *ScheduleAvailabilityList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil ScheduleAvailability rows")
	}
	*l = ScheduleAvailabilityList{}
	for rows.Next() {
		var item ScheduleAvailability
		var isActive int
		err := rows.Scan(&item.ID, &item.EmployeeID, &item.DayOfWeek, &item.StartTime, &item.EndTime,
			&item.StartDate, &item.EndDate, &isActive, &item.CreatedAt, &item.UpdatedAt, &item.EmployeeName)
		if err != nil {
			continue
		}
		item.IsActive = isActive == 1
		*l = append(*l, item)
	}
	return nil
}

func scheduleAvailabilitySelect() string {
	return `SELECT sa.id, sa.employee_id, sa.day_of_week, sa.start_time, sa.end_time,
		sa.start_date, sa.end_date, sa.is_active, sa.created_at, sa.updated_at,
		e.first_name || ' ' || e.last_name`
}

// ActiveSchedulesForWeek returns the active schedule_availability rows that
// cover the given week (started on/before weekEnd and not yet ended at
// weekStart). employeeID == "" returns every employee's.
func ActiveSchedulesForWeek(employeeID string, weekStart, weekEnd Date) ([]ScheduleAvailability, error) {
	where := ` WHERE sa.is_active = 1 AND sa.start_date <= ? AND (sa.end_date = '' OR sa.end_date >= ?)`
	args := []any{weekEnd, weekStart}
	if employeeID != "" {
		where += ` AND sa.employee_id = ?`
		args = append(args, employeeID)
	}
	rows, err := RDB.Query(scheduleAvailabilitySelect()+`
		FROM schedule_availability sa
		JOIN employees e ON e.id = sa.employee_id`+where+`
		ORDER BY e.first_name, sa.day_of_week, sa.start_time`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items ScheduleAvailabilityList
	if err := items.ScanRows(rows); err != nil {
		return nil, err
	}
	return items, rows.Err()
}

func (sa *ScheduleAvailability) GetByID(id string) error {
	return sa.ScanRow(RDB.QueryRow(scheduleAvailabilitySelect()+`
		FROM schedule_availability sa
		JOIN employees e ON e.id = sa.employee_id
		WHERE sa.id = ?`, id))
}

func (sa *ScheduleAvailability) Create() error {
	if err := sa.IsValid(); err != nil {
		return err
	}
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := sa.createWithTx(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (sa *ScheduleAvailability) createWithTx(tx *sql.Tx) error {
	sa.ID = uuid.Must(uuid.NewV7()).String()
	now := DateNow()
	sa.CreatedAt = now
	sa.UpdatedAt = now
	sa.IsActive = true
	sa.normalizeDates()

	if _, err := tx.Exec(`UPDATE schedule_availability
		SET is_active = 0, end_date = ?, updated_at = ?
		WHERE employee_id = ? AND day_of_week = ? AND is_active = 1`,
		sa.StartDate, now, sa.EmployeeID, sa.DayOfWeek); err != nil {
		return err
	}

	_, err := tx.Exec(`INSERT INTO schedule_availability (`+scheduleAvailabilityColumns+`)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		sa.ID, sa.EmployeeID, sa.DayOfWeek, sa.StartTime, sa.EndTime,
		sa.StartDate, sa.EndDate, BoolToInt(sa.IsActive), sa.CreatedAt, sa.UpdatedAt)
	return err
}

func (sa *ScheduleAvailability) Update(updates map[string]any) error {
	var current ScheduleAvailability
	if err := current.GetByID(sa.ID); err != nil {
		return err
	}

	if current.IsActive && scheduleAvailabilityNeedsVersion(updates) {
		next := current
		next.ID = ""
		if v, ok := stringUpdate(updates, "startTime"); ok {
			next.StartTime = v
		}
		if v, ok := stringUpdate(updates, "endTime"); ok {
			next.EndTime = v
		}
		if v, ok := intUpdate(updates, "dayOfWeek"); ok {
			next.DayOfWeek = v
		}
		if v, ok := dateUpdate(updates, "startDate"); ok {
			next.StartDate = v
		}
		if v, ok := dateUpdate(updates, "endDate"); ok {
			next.EndDate = v
		} else {
			next.EndDate = ""
		}

		if err := next.IsValid(); err != nil {
			return err
		}
		tx, err := DB.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		now := DateNow()
		if _, err := tx.Exec(`UPDATE schedule_availability SET is_active = 0, end_date = ?, updated_at = ? WHERE id = ?`,
			next.StartDate, now, current.ID); err != nil {
			return err
		}
		if err := next.createWithTx(tx); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		*sa = next
		return nil
	}

	cols := map[string]string{
		"dayOfWeek": "day_of_week", "startTime": "start_time", "endTime": "end_time",
		"startDate": "start_date", "endDate": "end_date",
		"isActive": "is_active",
	}
	setClauses := ""
	var args []any
	for jsonKey, dbCol := range cols {
		val, ok := updates[jsonKey]
		if !ok {
			continue
		}
		if dbCol == "is_active" {
			b, ok := boolUpdate(updates, jsonKey)
			if !ok {
				return fmt.Errorf("invalid isActive")
			}
			val = BoolToInt(b)
		}
		if setClauses != "" {
			setClauses += ", "
		}
		setClauses += dbCol + " = ?"
		args = append(args, val)
	}
	if setClauses == "" {
		return sa.GetByID(sa.ID)
	}
	setClauses += ", updated_at = ?"
	args = append(args, DateNow(), sa.ID)
	if _, err := DB.Exec("UPDATE schedule_availability SET "+setClauses+" WHERE id = ?", args...); err != nil {
		return err
	}
	return sa.GetByID(sa.ID)
}

func scheduleAvailabilityNeedsVersion(updates map[string]any) bool {
	for _, key := range []string{"dayOfWeek", "startTime", "endTime", "startDate"} {
		if _, ok := updates[key]; ok {
			return true
		}
	}
	return false
}

func stringUpdate(updates map[string]any, key string) (string, bool) {
	v, ok := updates[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func dateUpdate(updates map[string]any, key string) (Date, bool) {
	if s, ok := stringUpdate(updates, key); ok {
		return Date(s), true
	}
	return "", false
}

func intUpdate(updates map[string]any, key string) (int, bool) {
	v, ok := updates[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	default:
		return 0, false
	}
}

func boolUpdate(updates map[string]any, key string) (bool, bool) {
	v, ok := updates[key]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

func (sa *ScheduleAvailability) Delete() error {
	res, err := DB.Exec("DELETE FROM schedule_availability WHERE id = ?", sa.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
