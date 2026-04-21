package store

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

const scheduleAvailabilityColumnsNoId = `employee_id, day_of_week, start_time, end_time, effective_date, created_at`
const scheduleAvailabilityColumns = `id, ` + scheduleAvailabilityColumnsNoId

type ScheduleAvailability struct {
	ID            string `json:"id"`
	EmployeeID    string `json:"employeeId"`
	DayOfWeek     int    `json:"dayOfWeek"`
	StartTime     string `json:"startTime"`
	EndTime       string `json:"endTime"`
	EffectiveDate Date   `json:"effectiveDate"`
	CreatedAt     Date   `json:"createdAt"`
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
	if len(e) > 0 {
		return e
	}
	return nil
}

type ScheduleAvailabilityList []ScheduleAvailability

func (m *ScheduleAvailability) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil ScheduleAvailability row")
	}
	err := row.Scan(&m.ID, &m.EmployeeID, &m.DayOfWeek, &m.StartTime, &m.EndTime,
		&m.EffectiveDate, &m.CreatedAt, &m.EmployeeName)
	if err != nil {
		return err
	}
	return nil
}

func (l *ScheduleAvailabilityList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil ScheduleAvailability rows")
	}
	*l = ScheduleAvailabilityList{}
	for rows.Next() {
		var item ScheduleAvailability
		err := rows.Scan(&item.ID, &item.EmployeeID, &item.DayOfWeek, &item.StartTime, &item.EndTime,
			&item.EffectiveDate, &item.CreatedAt, &item.EmployeeName)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (sa *ScheduleAvailability) GetByEmployee(employeeID string) ([]ScheduleAvailability, error) {
	rows, err := RDB.Query(`SELECT sa.id, sa.employee_id, sa.day_of_week, sa.start_time, sa.end_time,
		sa.effective_date, sa.created_at,
		e.first_name || ' ' || e.last_name
		FROM schedule_availability sa
		JOIN employees e ON e.id = sa.employee_id
		WHERE sa.employee_id = ? ORDER BY sa.day_of_week, sa.start_time`, employeeID)
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

func (sa *ScheduleAvailability) GetAll(params ListParams) ([]ScheduleAvailability, int, error) {
	baseFrom := ` FROM schedule_availability sa
		JOIN employees e ON e.id = sa.employee_id`
	where := ""
	var args []any
	if fc, fa := params.FilterClause("e.first_name", "e.last_name"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*)"+baseFrom+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `SELECT sa.id, sa.employee_id, sa.day_of_week, sa.start_time, sa.end_time,
		sa.effective_date, sa.created_at,
		e.first_name || ' ' || e.last_name` + baseFrom + where + ` ORDER BY e.first_name, sa.day_of_week, sa.start_time` + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items ScheduleAvailabilityList
	if err := items.ScanRows(rows); err != nil {
		return nil, 0, err
	}
	return items, total, rows.Err()
}

func (sa *ScheduleAvailability) Create() error {
	sa.ID = uuid.Must(uuid.NewV7()).String()
	sa.CreatedAt = DateNow()
	_, err := DB.Exec(`INSERT INTO schedule_availability (`+scheduleAvailabilityColumns+`)
		VALUES (?,?,?,?,?,?,?)`,
		sa.ID, sa.EmployeeID, sa.DayOfWeek, sa.StartTime, sa.EndTime, sa.EffectiveDate, sa.CreatedAt)
	return err
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
