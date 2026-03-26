package models

import (
	"database/sql"

	"github.com/google/uuid"
)

type ScheduleAvailability struct {
	ID            string `json:"id"`
	EmployeeID    string `json:"employeeId"`
	DayOfWeek     int    `json:"dayOfWeek"`
	StartTime     string `json:"startTime"`
	EndTime       string `json:"endTime"`
	IsAvailable   bool   `json:"isAvailable"`
	EffectiveDate string `json:"effectiveDate"`
	CreatedAt     string `json:"createdAt"`
	// Joined fields
	EmployeeName string `json:"employeeName,omitempty"`
}

func (sa *ScheduleAvailability) GetByEmployee(employeeID string) ([]ScheduleAvailability, error) {
	rows, err := DB.Query(`SELECT sa.id, sa.employee_id, sa.day_of_week, sa.start_time, sa.end_time,
		sa.is_available, sa.effective_date, sa.created_at,
		tm.first_name || ' ' || tm.last_name
		FROM schedule_availability sa
		JOIN team_members tm ON tm.id = sa.employee_id
		WHERE sa.employee_id = ? ORDER BY sa.day_of_week, sa.start_time`, employeeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanScheduleAvailabilityRows(rows)
}

func (sa *ScheduleAvailability) GetAll() ([]ScheduleAvailability, error) {
	rows, err := DB.Query(`SELECT sa.id, sa.employee_id, sa.day_of_week, sa.start_time, sa.end_time,
		sa.is_available, sa.effective_date, sa.created_at,
		tm.first_name || ' ' || tm.last_name
		FROM schedule_availability sa
		JOIN team_members tm ON tm.id = sa.employee_id
		ORDER BY tm.first_name, sa.day_of_week, sa.start_time`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanScheduleAvailabilityRows(rows)
}

func (sa *ScheduleAvailability) Create() error {
	sa.ID = uuid.New().String()
	_, err := DB.Exec(`INSERT INTO schedule_availability (id, employee_id, day_of_week, start_time, end_time, is_available, effective_date, created_at)
		VALUES (?,?,?,?,?,?,?,?)`,
		sa.ID, sa.EmployeeID, sa.DayOfWeek, sa.StartTime, sa.EndTime, BoolToInt(sa.IsAvailable), sa.EffectiveDate, sa.CreatedAt)
	return err
}

func (sa *ScheduleAvailability) Delete() error {
	res, err := DB.Exec("DELETE FROM schedule_availability WHERE id = ?", sa.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func scanScheduleAvailabilityRows(rows *sql.Rows) ([]ScheduleAvailability, error) {
	var items []ScheduleAvailability
	for rows.Next() {
		var sa ScheduleAvailability
		var isAvailable int
		if err := rows.Scan(&sa.ID, &sa.EmployeeID, &sa.DayOfWeek, &sa.StartTime, &sa.EndTime,
			&isAvailable, &sa.EffectiveDate, &sa.CreatedAt, &sa.EmployeeName); err != nil {
			return nil, err
		}
		sa.IsAvailable = isAvailable == 1
		items = append(items, sa)
	}
	return items, rows.Err()
}
