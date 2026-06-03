package store

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
)

const employeeScheduleColumnsNoId = `employee_id, day_of_week, start_time, end_time, start_date, end_date, is_active, created_at, updated_at`
const employeeScheduleColumns = `id, ` + employeeScheduleColumnsNoId

type EmployeeSchedule struct {
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

func (sa *EmployeeSchedule) IsValid() error {
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

func (sa *EmployeeSchedule) normalizeDates() {
	if sa.StartDate.IsZero() {
		sa.StartDate = ClinicToday()
	}
}

type EmployeeScheduleList []EmployeeSchedule

func (m *EmployeeSchedule) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil EmployeeSchedule row")
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

func (l *EmployeeScheduleList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil EmployeeSchedule rows")
	}
	*l = EmployeeScheduleList{}
	for rows.Next() {
		var item EmployeeSchedule
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

func employeeScheduleSelect() string {
	return `SELECT sa.id, sa.employee_id, sa.day_of_week, sa.start_time, sa.end_time,
		sa.start_date, sa.end_date, sa.is_active, sa.created_at, sa.updated_at,
		e.first_name || ' ' || e.last_name`
}

// ActiveSchedulesForWeek returns the active employee_schedules rows that
// cover the given week (started on/before weekEnd and not yet ended at
// weekStart). employeeID == "" returns every employee's.
func ActiveSchedulesForWeek(employeeID string, weekStart, weekEnd Date) ([]EmployeeSchedule, error) {
	where := ` WHERE sa.is_active = 1 AND sa.start_date <= ? AND (sa.end_date = '' OR sa.end_date >= ?)`
	args := []any{weekEnd, weekStart}
	if employeeID != "" {
		where += ` AND sa.employee_id = ?`
		args = append(args, employeeID)
	}
	rows, err := RDB.Query(employeeScheduleSelect()+`
		FROM employee_schedules sa
		JOIN employees e ON e.id = sa.employee_id`+where+`
		ORDER BY e.first_name, sa.day_of_week, sa.start_time`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items EmployeeScheduleList
	if err := items.ScanRows(rows); err != nil {
		return nil, err
	}
	return items, rows.Err()
}

func (sa *EmployeeSchedule) GetByID(id string) error {
	return sa.ScanRow(RDB.QueryRow(employeeScheduleSelect()+`
		FROM employee_schedules sa
		JOIN employees e ON e.id = sa.employee_id
		WHERE sa.id = ?`, id))
}

func (sa *EmployeeSchedule) Create() error {
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

func (sa *EmployeeSchedule) createWithTx(tx *sql.Tx) error {
	sa.ID = uuid.Must(uuid.NewV7()).String()
	now := DateNow()
	sa.CreatedAt = now
	sa.UpdatedAt = now
	sa.IsActive = true
	sa.normalizeDates()

	if _, err := tx.Exec(`UPDATE employee_schedules
		SET is_active = 0, end_date = ?, updated_at = ?
		WHERE employee_id = ? AND day_of_week = ? AND is_active = 1`,
		sa.StartDate, now, sa.EmployeeID, sa.DayOfWeek); err != nil {
		return err
	}

	_, err := tx.Exec(`INSERT INTO employee_schedules (`+employeeScheduleColumns+`)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		sa.ID, sa.EmployeeID, sa.DayOfWeek, sa.StartTime, sa.EndTime,
		sa.StartDate, sa.EndDate, BoolToInt(sa.IsActive), sa.CreatedAt, sa.UpdatedAt)
	return err
}

func (sa *EmployeeSchedule) Update(updates map[string]any) error {
	var current EmployeeSchedule
	if err := current.GetByID(sa.ID); err != nil {
		return err
	}

	if current.IsActive && employeeScheduleNeedsVersion(updates) {
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
		if _, err := tx.Exec(`UPDATE employee_schedules SET is_active = 0, end_date = ?, updated_at = ? WHERE id = ?`,
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
	if _, err := DB.Exec("UPDATE employee_schedules SET "+setClauses+" WHERE id = ?", args...); err != nil {
		return err
	}
	return sa.GetByID(sa.ID)
}

func employeeScheduleNeedsVersion(updates map[string]any) bool {
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

func (sa *EmployeeSchedule) Delete() error {
	res, err := DB.Exec("DELETE FROM employee_schedules WHERE id = ?", sa.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// A contiguous block of working time. Kind is "regular" or "overtime".
type ScheduleShift struct {
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
	Kind      string `json:"kind,omitempty"`
}

type EmployeeScheduleDay struct {
	EmployeeID    string          `json:"employeeId"`
	EmployeeName  string          `json:"employeeName,omitempty"`
	WorkDate      Date            `json:"workDate"`
	DayOfWeek     int             `json:"dayOfWeek"`
	Shifts        []ScheduleShift `json:"shifts"`
	IsOff         bool            `json:"isOff"`
	OffReason     string          `json:"offReason,omitempty"`
	Hours         float64         `json:"hours"`
	OvertimeHours float64         `json:"overtimeHours"`
}

// EmployeeScheduleForRange projects each employee's template across [from, to],
// adding accepted overtime and removing holidays and accepted timeoff.
// employeeID == "" projects every employee, one row per employee per day.
func EmployeeScheduleForRange(employeeID string, from Date, to Date) ([]EmployeeScheduleDay, error) {
	start, end, err := parseDateRange(from, to)
	if err != nil {
		return nil, err
	}
	type employeeRow struct{ id, name string }
	var employees []employeeRow
	if employeeID != "" {
		var name string
		if err := RDB.QueryRow(`SELECT first_name || ' ' || last_name FROM employees WHERE id = ?`, employeeID).Scan(&name); err != nil {
			return nil, err
		}
		employees = []employeeRow{{employeeID, name}}
	} else {
		rows, err := RDB.Query(`SELECT id, first_name || ' ' || last_name FROM employees ORDER BY first_name, last_name`)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var e employeeRow
			if err := rows.Scan(&e.id, &e.name); err != nil {
				rows.Close()
				return nil, err
			}
			employees = append(employees, e)
		}
		rows.Close()
	}

	holidays, err := holidayMapForRange(start, end)
	if err != nil {
		return nil, err
	}

	var out []EmployeeScheduleDay
	for _, emp := range employees {
		changes, err := acceptedScheduleChangesForRange(emp.id, from, to)
		if err != nil {
			return nil, err
		}
		for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
			workDate := Date(d.Format(DateFormat))
			day := EmployeeScheduleDay{
				EmployeeID:   emp.id,
				EmployeeName: emp.name,
				WorkDate:     workDate,
				DayOfWeek:    int(d.Weekday()),
				Shifts:       []ScheduleShift{},
			}

			startTime, endTime, hasShift, err := generalScheduleForDate(emp.id, d)
			if err != nil {
				return nil, err
			}
			_, isHoliday := holidays[workDate]

			var regularShifts []ScheduleShift
			if hasShift && !isHoliday {
				regularShifts = []ScheduleShift{{StartTime: startTime, EndTime: endTime, Kind: "regular"}}
			}
			var overtimeShifts []ScheduleShift
			for _, ch := range changes {
				if !changeAppliesToDay(ch, workDate) {
					continue
				}
				if ch.Type == "overtime" {
					overtimeShifts = append(overtimeShifts, ScheduleShift{
						StartTime: ch.StartTime, EndTime: ch.EndTime, Kind: "overtime",
					})
				}
			}
			for _, ch := range changes {
				if ch.Type != "timeoff" || !changeAppliesToDay(ch, workDate) {
					continue
				}
				offStart, offEnd, applies := timeoffWindow(ch)
				if !applies {
					continue
				}
				regularShifts = subtractInterval(regularShifts, offStart, offEnd)
				overtimeShifts = subtractInterval(overtimeShifts, offStart, offEnd)
			}

			shifts := append(regularShifts, overtimeShifts...)
			sortShiftsByStart(shifts)
			day.Shifts = shifts

			for _, s := range regularShifts {
				day.Hours += scheduleHours(s.StartTime, s.EndTime)
			}
			for _, s := range overtimeShifts {
				h := scheduleHours(s.StartTime, s.EndTime)
				day.Hours += h
				day.OvertimeHours += h
			}
			day.Hours = math.Round(day.Hours*100) / 100
			day.OvertimeHours = math.Round(day.OvertimeHours*100) / 100

			day.IsOff = len(shifts) == 0
			if day.IsOff {
				switch {
				case !hasShift:
					day.OffReason = "no-schedule"
				case isHoliday:
					day.OffReason = "holiday"
				default:
					day.OffReason = "timeoff"
				}
			}
			out = append(out, day)
		}
	}
	return out, nil
}

// Returns the active template shift for date d, or hasShift=false if none.
func generalScheduleForDate(employeeID string, d time.Time) (string, string, bool, error) {
	day := int(d.Weekday())
	workDate := Date(d.Format(DateFormat))
	var startTime, endTime string
	err := RDB.QueryRow(`SELECT start_time, end_time
		FROM employee_schedules
		WHERE employee_id = ? AND day_of_week = ? AND start_date <= ? AND (end_date = '' OR end_date > ?)
		ORDER BY start_date DESC, created_at DESC
		LIMIT 1`, employeeID, day, workDate, workDate).Scan(&startTime, &endTime)
	if err == sql.ErrNoRows {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return startTime, endTime, true, nil
}

func holidayMapForRange(start time.Time, end time.Time) (map[Date]string, error) {
	rows, err := RDB.Query(`SELECT start_date, end_date, name FROM holidays
		WHERE end_date >= ? AND start_date <= ?`,
		start.Format(DateFormat), end.Format(DateFormat))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[Date]string{}
	for rows.Next() {
		var hStart, hEnd Date
		var name string
		if err := rows.Scan(&hStart, &hEnd, &name); err != nil {
			return nil, err
		}
		// Parse the calendar-day prefix as UTC midnight so range expansion
		// stays on the sender's intended day regardless of stored timezone.
		hs, err := time.Parse(DateFormat, hStart.DateOnly())
		if err != nil {
			continue
		}
		he, err := time.Parse(DateFormat, hEnd.DateOnly())
		if err != nil {
			continue
		}
		if hs.Before(start) {
			hs = start
		}
		if he.After(end) {
			he = end
		}
		for d := hs; !d.After(he); d = d.AddDate(0, 0, 1) {
			out[Date(d.Format(DateFormat))] = name
		}
	}
	return out, rows.Err()
}

func acceptedScheduleChangesForRange(employeeID string, from Date, to Date) ([]EmployeeScheduleChange, error) {
	rows, err := RDB.Query(`SELECT `+employeeScheduleChangeColumns+` FROM employee_schedule_changes
		WHERE employee_id = ? AND status = 'accepted' AND end_date >= ? AND start_date <= ?
		ORDER BY start_date`, employeeID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EmployeeScheduleChange
	for rows.Next() {
		var v EmployeeScheduleChange
		if err := rows.Scan(&v.ID, &v.EmployeeID, &v.Type, &v.StartDate, &v.EndDate, &v.StartTime, &v.EndTime,
			&v.Status, &v.Notes, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func changeAppliesToDay(ch EmployeeScheduleChange, day Date) bool {
	return !day.Before(ch.StartDate) && !ch.EndDate.Before(day)
}

// Minutes-of-day window a timeoff covers. Full-day off (no times) is [0, 1440].
func timeoffWindow(v EmployeeScheduleChange) (int, int, bool) {
	if v.StartTime == "" && v.EndTime == "" {
		return 0, 1440, true
	}
	startMin, endMin := 0, 1440
	if v.StartTime != "" {
		if m, ok := minutesOfDay(v.StartTime); ok {
			startMin = m
		}
	}
	if v.EndTime != "" {
		if m, ok := minutesOfDay(v.EndTime); ok {
			endMin = m
		}
	}
	if endMin <= startMin {
		return 0, 0, false
	}
	return startMin, endMin, true
}

func sortShiftsByStart(shifts []ScheduleShift) {
	for i := 1; i < len(shifts); i++ {
		j := i
		for j > 0 {
			a, _ := minutesOfDay(shifts[j-1].StartTime)
			b, _ := minutesOfDay(shifts[j].StartTime)
			if a <= b {
				break
			}
			shifts[j-1], shifts[j] = shifts[j], shifts[j-1]
			j--
		}
	}
}

// subtractInterval removes [offStart, offEnd] (minutes-of-day) from each shift,
// returning the remaining shift segments in order.
func subtractInterval(shifts []ScheduleShift, offStart int, offEnd int) []ScheduleShift {
	out := make([]ScheduleShift, 0, len(shifts))
	for _, s := range shifts {
		shiftStart, ok1 := minutesOfDay(s.StartTime)
		shiftEnd, ok2 := minutesOfDay(s.EndTime)
		if !ok1 || !ok2 || shiftEnd <= shiftStart {
			continue
		}
		// no overlap
		if offEnd <= shiftStart || offStart >= shiftEnd {
			out = append(out, s)
			continue
		}
		// off interval covers the shift entirely
		if offStart <= shiftStart && offEnd >= shiftEnd {
			continue
		}
		// trim left
		if offStart > shiftStart {
			out = append(out, ScheduleShift{StartTime: s.StartTime, EndTime: minutesToClock(offStart), Kind: s.Kind})
		}
		// trim right
		if offEnd < shiftEnd {
			out = append(out, ScheduleShift{StartTime: minutesToClock(offEnd), EndTime: s.EndTime, Kind: s.Kind})
		}
	}
	return out
}

func minutesOfDay(clock string) (int, bool) {
	t, err := parseClock(clock)
	if err != nil {
		return 0, false
	}
	return t.Hour()*60 + t.Minute(), true
}

func minutesToClock(m int) string {
	if m < 0 {
		m = 0
	}
	if m > 1440 {
		m = 1440
	}
	return fmt.Sprintf("%02d:%02d", m/60, m%60)
}

func parseDateRange(from Date, to Date) (time.Time, time.Time, error) {
	start, err := from.Time()
	if err != nil || start.IsZero() {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid period start")
	}
	end, err := to.Time()
	if err != nil || end.IsZero() {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid period end")
	}
	if end.Before(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("period end must be after period start")
	}
	return start, end, nil
}

func scheduleHours(start string, end string) float64 {
	if start == "" || end == "" {
		return 0
	}
	startTime, err := parseClock(start)
	if err != nil {
		return 0
	}
	endTime, err := parseClock(end)
	if err != nil {
		return 0
	}
	if !endTime.After(startTime) {
		return 0
	}
	return math.Round(endTime.Sub(startTime).Hours()*100) / 100
}

func parseClock(value string) (time.Time, error) {
	for _, layout := range []string{"15:04", "15:04:05"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid time: %s", value)
}
