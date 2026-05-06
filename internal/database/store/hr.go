package store

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"clinic-api/internal/validation"

	"github.com/google/uuid"
)

// ============================================================
// HOLIDAYS
// ============================================================

type Holiday struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	StartDate Date   `json:"startDate"`
	EndDate   Date   `json:"endDate"`
	Notes     string `json:"notes"`
	CreatedBy string `json:"createdBy"`
	CreatedAt Date   `json:"createdAt"`
	UpdatedAt Date   `json:"updatedAt"`
}

const holidayColumnsNoId = `name, start_date, end_date, notes, created_by, created_at, updated_at`
const holidayColumns = `id, ` + holidayColumnsNoId

type HolidayList []Holiday

func (h *Holiday) IsValid() error {
	if h.Name == "" {
		return fmt.Errorf("name is required")
	}
	if h.StartDate.IsZero() || h.EndDate.IsZero() {
		return fmt.Errorf("startDate and endDate are required")
	}
	if h.EndDate.Before(h.StartDate) {
		return fmt.Errorf("endDate must be on or after startDate")
	}
	return nil
}

func (h *Holiday) scanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil Holiday row")
	}
	return row.Scan(&h.ID, &h.Name, &h.StartDate, &h.EndDate, &h.Notes, &h.CreatedBy, &h.CreatedAt, &h.UpdatedAt)
}

func (l *HolidayList) GetAll(params ListParams) (int, error) {
	where := ""
	var args []any
	if fc, fa := params.FilterClause("name", "notes"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}
	var total int
	if err := RDB.QueryRow("SELECT COUNT(*) FROM holidays"+where, args...).Scan(&total); err != nil {
		return 0, err
	}
	rows, err := RDB.Query(`SELECT `+holidayColumns+` FROM holidays`+where+` ORDER BY start_date DESC`+params.PaginationClause(), args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	*l = HolidayList{}
	for rows.Next() {
		var item Holiday
		if err := rows.Scan(&item.ID, &item.Name, &item.StartDate, &item.EndDate, &item.Notes, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt); err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return total, rows.Err()
}

// HolidaysOverlappingWeek returns holidays whose date range intersects the
// given week. Used by the unified weekly schedule endpoint.
func HolidaysOverlappingWeek(weekStart, weekEnd Date) (HolidayList, error) {
	rows, err := RDB.Query(`SELECT `+holidayColumns+` FROM holidays
		WHERE end_date >= ? AND start_date <= ?
		ORDER BY start_date`, weekStart, weekEnd)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items HolidayList
	for rows.Next() {
		var h Holiday
		if err := rows.Scan(&h.ID, &h.Name, &h.StartDate, &h.EndDate, &h.Notes, &h.CreatedBy, &h.CreatedAt, &h.UpdatedAt); err != nil {
			continue
		}
		items = append(items, h)
	}
	return items, rows.Err()
}

func (h *Holiday) GetByID(id string) error {
	return h.scanRow(RDB.QueryRow(`SELECT `+holidayColumns+` FROM holidays WHERE id = ?`, id))
}

func (h *Holiday) Create() error {
	h.StartDate = Date(h.StartDate.DateOnly())
	h.EndDate = Date(h.EndDate.DateOnly())
	if err := h.IsValid(); err != nil {
		return err
	}
	now := DateNow()
	h.ID = uuid.Must(uuid.NewV7()).String()
	h.CreatedAt = now
	h.UpdatedAt = now
	_, err := DB.Exec(`INSERT INTO holidays (`+holidayColumns+`) VALUES (?,?,?,?,?,?,?,?)`,
		h.ID, h.Name, h.StartDate, h.EndDate, h.Notes, h.CreatedBy, h.CreatedAt, h.UpdatedAt)
	return err
}

func (h *Holiday) Update(updates map[string]any) error {
	var current Holiday
	current.ID = h.ID
	if err := current.scanRow(RDB.QueryRow(`SELECT `+holidayColumns+` FROM holidays WHERE id = ?`, h.ID)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	next := current
	if v, ok := stringUpdate(updates, "name"); ok {
		next.Name = v
	}
	if v, ok := dateUpdate(updates, "startDate"); ok {
		next.StartDate = v
	}
	if v, ok := dateUpdate(updates, "endDate"); ok {
		next.EndDate = v
	}
	if v, ok := stringUpdate(updates, "notes"); ok {
		next.Notes = v
	}
	next.StartDate = Date(next.StartDate.DateOnly())
	next.EndDate = Date(next.EndDate.DateOnly())
	if err := next.IsValid(); err != nil {
		return err
	}
	now := DateNow()
	if _, err := DB.Exec(`UPDATE holidays SET name = ?, start_date = ?, end_date = ?, notes = ?, updated_at = ? WHERE id = ?`,
		next.Name, next.StartDate, next.EndDate, next.Notes, now, next.ID); err != nil {
		return err
	}
	next.UpdatedAt = now
	*h = next
	return nil
}

func (h *Holiday) Delete() error {
	res, err := DB.Exec(`DELETE FROM holidays WHERE id = ?`, h.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ============================================================
// EMPLOYEE VACATIONS
// ============================================================

type EmployeeVacation struct {
	ID           string `json:"id"`
	EmployeeID   string `json:"employeeId"`
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

var EmployeeVacationStatuses = []string{"pending", "accepted", "rejected"}

const employeeVacationColumnsNoId = `employee_id, start_date, end_date, start_time, end_time, status, notes, created_at, updated_at`
const employeeVacationColumns = `id, ` + employeeVacationColumnsNoId

type EmployeeVacationList []EmployeeVacation

func (v *EmployeeVacation) IsValid() error {
	e := make(validation.Errors)
	if msg := validation.Required(v.EmployeeID, "Employee ID"); msg != "" {
		e["employeeId"] = msg
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
	if v.StartTime != "" && v.StartDate != v.EndDate {
		e["time"] = "Partial-day vacation must use the same start and end date"
	}
	if v.Status != "" {
		if msg := validation.OneOf(v.Status, EmployeeVacationStatuses, "Status"); msg != "" {
			e["status"] = msg
		}
	}
	if len(e) > 0 {
		return e
	}
	return nil
}

func employeeVacationSelect() string {
	return `SELECT ev.id, ev.employee_id, ev.start_date, ev.end_date, ev.start_time, ev.end_time,
		ev.status, ev.notes, ev.created_at, ev.updated_at,
		e.first_name || ' ' || e.last_name`
}

func (v *EmployeeVacation) scanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil EmployeeVacation row")
	}
	return row.Scan(&v.ID, &v.EmployeeID, &v.StartDate, &v.EndDate, &v.StartTime, &v.EndTime,
		&v.Status, &v.Notes, &v.CreatedAt, &v.UpdatedAt, &v.EmployeeName)
}

func (l *EmployeeVacationList) scanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil EmployeeVacation rows")
	}
	*l = EmployeeVacationList{}
	for rows.Next() {
		var item EmployeeVacation
		if err := rows.Scan(&item.ID, &item.EmployeeID, &item.StartDate, &item.EndDate, &item.StartTime, &item.EndTime,
			&item.Status, &item.Notes, &item.CreatedAt, &item.UpdatedAt, &item.EmployeeName); err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (v *EmployeeVacation) GetByID(id string) error {
	return v.scanRow(RDB.QueryRow(employeeVacationSelect()+`
		FROM employee_vacations ev
		JOIN employees e ON e.id = ev.employee_id
		WHERE ev.id = ?`, id))
}

// VacationsOverlappingWeek returns vacations whose date range intersects the
// given week, regardless of status. employeeID == "" returns every employee's.
func VacationsOverlappingWeek(employeeID string, weekStart, weekEnd Date) (EmployeeVacationList, error) {
	where := ` WHERE ev.end_date >= ? AND ev.start_date <= ?`
	args := []any{weekStart, weekEnd}
	if employeeID != "" {
		where += ` AND ev.employee_id = ?`
		args = append(args, employeeID)
	}
	rows, err := RDB.Query(employeeVacationSelect()+`
		FROM employee_vacations ev
		JOIN employees e ON e.id = ev.employee_id`+where+`
		ORDER BY ev.start_date, e.first_name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items EmployeeVacationList
	if err := items.scanRows(rows); err != nil {
		return nil, err
	}
	return items, rows.Err()
}

func (v *EmployeeVacation) Create() error {
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
	_, err := DB.Exec(`INSERT INTO employee_vacations (`+employeeVacationColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		v.ID, v.EmployeeID, v.StartDate, v.EndDate, v.StartTime, v.EndTime, v.Status, v.Notes, v.CreatedAt, v.UpdatedAt)
	if err != nil {
		return err
	}
	return v.GetByID(v.ID)
}

func (v *EmployeeVacation) Update(updates map[string]any) error {
	var current EmployeeVacation
	if err := current.GetByID(v.ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	next := current
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
	if _, err := DB.Exec(`UPDATE employee_vacations SET start_date = ?, end_date = ?, start_time = ?, end_time = ?, notes = ?, updated_at = ? WHERE id = ?`,
		next.StartDate, next.EndDate, next.StartTime, next.EndTime, next.Notes, now, next.ID); err != nil {
		return err
	}
	next.UpdatedAt = now
	*v = next
	return nil
}

func (v *EmployeeVacation) Delete() error {
	res, err := DB.Exec(`DELETE FROM employee_vacations WHERE id = ?`, v.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (v *EmployeeVacation) SetStatus(status string) error {
	if msg := validation.OneOf(status, EmployeeVacationStatuses, "Status"); msg != "" {
		return validation.Errors{"status": msg}
	}
	now := DateNow()
	res, err := DB.Exec(`UPDATE employee_vacations SET status = ?, updated_at = ? WHERE id = ?`, status, now, v.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return v.GetByID(v.ID)
}

// ============================================================
// EMPLOYEE SCHEDULE PROJECTION
// ============================================================

type ScheduleShift struct {
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
}

type EmployeeScheduleDay struct {
	EmployeeID   string          `json:"employeeId"`
	EmployeeName string          `json:"employeeName,omitempty"`
	WorkDate     Date            `json:"workDate"`
	DayOfWeek    int             `json:"dayOfWeek"`
	Shifts       []ScheduleShift `json:"shifts"`
	IsOff        bool            `json:"isOff"`
	OffReason    string          `json:"offReason,omitempty"`
	Hours        float64         `json:"hours"`
}

// EmployeeScheduleForRange projects each employee's schedule_availability template
// across [from, to], removing time covered by holidays or employee_vacations.
// employeeID == "" projects every employee. The result is grouped per employee
// per day, sorted by employee name then date.
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
		vacations, err := vacationsForEmployeeRange(emp.id, from, to)
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
			}

			startTime, endTime, hasShift, err := generalScheduleForDate(emp.id, d)
			if err != nil {
				return nil, err
			}
			if !hasShift {
				day.IsOff = true
				day.OffReason = "no-schedule"
				day.Shifts = []ScheduleShift{}
				out = append(out, day)
				continue
			}
			if _, holiday := holidays[workDate]; holiday {
				day.IsOff = true
				day.OffReason = "holiday"
				day.Shifts = []ScheduleShift{}
				out = append(out, day)
				continue
			}

			shifts := []ScheduleShift{{StartTime: startTime, EndTime: endTime}}
			for _, vac := range vacations {
				offStart, offEnd, applies := vacationOffWindow(vac, workDate)
				if !applies {
					continue
				}
				shifts = subtractInterval(shifts, offStart, offEnd)
				if len(shifts) == 0 {
					break
				}
			}
			day.Shifts = shifts
			day.IsOff = len(shifts) == 0
			if day.IsOff {
				day.OffReason = "vacation"
			}
			for _, s := range shifts {
				day.Hours += scheduleHours(s.StartTime, s.EndTime)
			}
			day.Hours = math.Round(day.Hours*100) / 100
			out = append(out, day)
		}
	}
	return out, nil
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

func vacationsForEmployeeRange(employeeID string, from Date, to Date) ([]EmployeeVacation, error) {
	rows, err := RDB.Query(`SELECT `+employeeVacationColumns+` FROM employee_vacations
		WHERE employee_id = ? AND status = 'accepted' AND end_date >= ? AND start_date <= ?
		ORDER BY start_date`, employeeID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EmployeeVacation
	for rows.Next() {
		var v EmployeeVacation
		if err := rows.Scan(&v.ID, &v.EmployeeID, &v.StartDate, &v.EndDate, &v.StartTime, &v.EndTime,
			&v.Status, &v.Notes, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// vacationOffWindow returns the [start, end] minutes-of-day window that the
// vacation covers on `day`. If the vacation does not apply to `day`, applies=false.
// Full-day off is represented as [0, 1440] (one full day in minutes).
func vacationOffWindow(v EmployeeVacation, day Date) (int, int, bool) {
	if day.Before(v.StartDate) || v.EndDate.Before(day) {
		return 0, 0, false
	}
	if v.StartTime == "" && v.EndTime == "" {
		return 0, 1440, true
	}
	// Partial-day vacations are required by IsValid to have start_date == end_date.
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
			out = append(out, ScheduleShift{StartTime: s.StartTime, EndTime: minutesToClock(offStart)})
		}
		// trim right
		if offEnd < shiftEnd {
			out = append(out, ScheduleShift{StartTime: minutesToClock(offEnd), EndTime: s.EndTime})
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

// generalScheduleForDate returns the active schedule_availability shift for an
// employee on date d, or hasShift=false if no template covers that day.
func generalScheduleForDate(employeeID string, d time.Time) (string, string, bool, error) {
	day := int(d.Weekday())
	workDate := Date(d.Format(DateFormat))
	var startTime, endTime string
	err := RDB.QueryRow(`SELECT start_time, end_time
		FROM schedule_availability
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

// ============================================================
// SALARY PREPARATION
// ============================================================

// EmployeeSalaryPreparation is one employee's prepared salary for one period.
// Each Prepare run inserts one row per eligible employee, all sharing the
// same period and createdBy. There is no separate "run" table; the run is
// just the set of rows with the same (period_start, period_end).
type EmployeeSalaryPreparation struct {
	ID             string  `json:"id"`
	EmployeeID     string  `json:"employeeId"`
	EmployeeName   string  `json:"employeeName,omitempty"`
	PeriodStart    Date    `json:"periodStart"`
	PeriodEnd      Date    `json:"periodEnd"`
	SalaryID       string  `json:"salaryId"`
	TransactionID  string  `json:"transactionId"`
	CurrencyID     string  `json:"currencyId"`
	BaseSalary     float64 `json:"baseSalary"`
	Adjustment     float64 `json:"adjustment"`
	PreparedAmount float64 `json:"preparedAmount"`
	Notes          string  `json:"notes"`
	CreatedBy      string  `json:"createdBy"`
	CreatedAt      Date    `json:"createdAt"`
}

const employeeSalaryPreparationColumns = `id, employee_id, period_start, period_end, salary_id, transaction_id, currency_id, base_salary, adjustment, prepared_amount, notes, created_by, created_at`

// PrepareEmployeeSalaries snapshots each employee's latest active salary for
// the period. Idempotent per employee: re-running for the same period skips
// employees that already have a preparation row and only inserts the missing
// ones. Returns the rows it created (possibly empty).
func PrepareEmployeeSalaries(periodStart Date, periodEnd Date, notes string, createdBy string) ([]EmployeeSalaryPreparation, error) {
	if _, _, err := parseDateRange(periodStart, periodEnd); err != nil {
		return nil, err
	}

	tx, err := DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	rows, err := tx.Query(`SELECT id, first_name || ' ' || last_name FROM employees ORDER BY first_name, last_name`)
	if err != nil {
		return nil, err
	}
	type employeeRow struct{ id, name string }
	var employees []employeeRow
	for rows.Next() {
		var e employeeRow
		if err := rows.Scan(&e.id, &e.name); err != nil {
			rows.Close()
			return nil, err
		}
		employees = append(employees, e)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	out := []EmployeeSalaryPreparation{}
	for _, e := range employees {
		var existing string
		err := tx.QueryRow(`SELECT id FROM employee_salary_preparations
			WHERE employee_id = ? AND period_start = ? AND period_end = ? LIMIT 1`,
			e.id, periodStart, periodEnd).Scan(&existing)
		if err == nil {
			continue
		}
		if err != sql.ErrNoRows {
			return nil, err
		}

		var salary EmployeeSalary
		err = tx.QueryRow(`SELECT `+employeeSalaryColumns+` FROM employee_salaries
			WHERE employee_id = ? AND effective_date <= ?
			ORDER BY effective_date DESC, created_at DESC LIMIT 1`, e.id, periodEnd).
			Scan(&salary.ID, &salary.EmployeeID, &salary.Amount, &salary.CurrencyID, new(int),
				&salary.EffectiveDate, &salary.Notes, &salary.CreatedAt, &salary.UpdatedAt)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return nil, err
		}
		if salary.Amount <= 0 {
			continue
		}

		selfBalance, err := getOrCreateBalanceWithTx(tx, "self", "self", "Clinic", salary.CurrencyID)
		if err != nil {
			return nil, err
		}
		employeeBalance, err := getOrCreateBalanceWithTx(tx, "employee", e.id, e.name, salary.CurrencyID)
		if err != nil {
			return nil, err
		}

		prep := EmployeeSalaryPreparation{
			ID:             uuid.Must(uuid.NewV7()).String(),
			EmployeeID:     e.id,
			EmployeeName:   e.name,
			PeriodStart:    periodStart,
			PeriodEnd:      periodEnd,
			SalaryID:       salary.ID,
			CurrencyID:     salary.CurrencyID,
			BaseSalary:     salary.Amount,
			PreparedAmount: salary.Amount,
			Notes:          notes,
			CreatedBy:      createdBy,
			CreatedAt:      DateNow(),
		}

		bt := BalanceTransaction{
			FromBalanceID:     employeeBalance.ID,
			ToBalanceID:       selfBalance.ID,
			Amount:            prep.PreparedAmount,
			CurrencyID:        prep.CurrencyID,
			TransactionType:   "charge",
			TransactionMethod: "other",
			SourceType:        "employee_salary_preparation",
			SourceID:          prep.ID,
			Description:       fmt.Sprintf("Salary preparation %s to %s", periodStart, periodEnd),
			CreatedBy:         createdBy,
		}
		if err := bt.CreateWithTx(tx); err != nil {
			return nil, err
		}
		prep.TransactionID = bt.ID

		if _, err := tx.Exec(`INSERT INTO employee_salary_preparations
			(`+employeeSalaryPreparationColumns+`)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			prep.ID, prep.EmployeeID, prep.PeriodStart, prep.PeriodEnd, prep.SalaryID, prep.TransactionID,
			prep.CurrencyID, prep.BaseSalary, prep.Adjustment, prep.PreparedAmount, prep.Notes, prep.CreatedBy, prep.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, prep)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

// PreparedSalariesForEmployee lists every prepared salary booked for the
// given employee, most recent period first.
func PreparedSalariesForEmployee(employeeID string) ([]EmployeeSalaryPreparation, error) {
	rows, err := RDB.Query(`SELECT `+employeeSalaryPreparationColumns+` FROM employee_salary_preparations
		WHERE employee_id = ?
		ORDER BY period_start DESC, created_at DESC`, employeeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EmployeeSalaryPreparation{}
	for rows.Next() {
		var p EmployeeSalaryPreparation
		if err := rows.Scan(&p.ID, &p.EmployeeID, &p.PeriodStart, &p.PeriodEnd, &p.SalaryID, &p.TransactionID,
			&p.CurrencyID, &p.BaseSalary, &p.Adjustment, &p.PreparedAmount, &p.Notes, &p.CreatedBy, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SetAdjustment updates the signed adjustment on a prepared salary, recomputes
// prepared_amount = base_salary + adjustment, and patches the linked balance
// transaction (and its two balances) by the resulting delta. The adjustment
// may be negative (a deduction) but the prepared amount must stay non-negative.
func (p *EmployeeSalaryPreparation) SetAdjustment(adjustment float64) error {
	adjustment = math.Round(adjustment*100) / 100

	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var current EmployeeSalaryPreparation
	err = tx.QueryRow(`SELECT `+employeeSalaryPreparationColumns+` FROM employee_salary_preparations WHERE id = ?`, p.ID).
		Scan(&current.ID, &current.EmployeeID, &current.PeriodStart, &current.PeriodEnd, &current.SalaryID, &current.TransactionID,
			&current.CurrencyID, &current.BaseSalary, &current.Adjustment, &current.PreparedAmount, &current.Notes, &current.CreatedBy, &current.CreatedAt)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	newAmount := math.Round((current.BaseSalary+adjustment)*100) / 100
	if newAmount < 0 {
		return fmt.Errorf("adjustment would make prepared amount negative")
	}
	delta := math.Round((newAmount-current.PreparedAmount)*100) / 100

	if delta != 0 && current.TransactionID != "" {
		var fromID, toID string
		if err := tx.QueryRow(`SELECT from_balance_id, to_balance_id FROM balance_transactions WHERE id = ?`, current.TransactionID).
			Scan(&fromID, &toID); err != nil {
			return err
		}
		now := DateNow()
		// transaction_type is "charge", so total_in / total_out are not touched.
		if _, err := tx.Exec(`UPDATE balance_transactions SET amount = ? WHERE id = ?`, newAmount, current.TransactionID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE balances SET amount = amount - ?, updated_at = ? WHERE id = ?`, delta, now, fromID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE balances SET amount = amount + ?, updated_at = ? WHERE id = ?`, delta, now, toID); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(`UPDATE employee_salary_preparations SET adjustment = ?, prepared_amount = ? WHERE id = ?`,
		adjustment, newAmount, p.ID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	current.Adjustment = adjustment
	current.PreparedAmount = newAmount
	*p = current
	return nil
}

// Delete reverses the linked balance transaction (if any) and removes the
// preparation row. Idempotent against partial state: a missing transaction
// row is treated as already reversed.
func (p *EmployeeSalaryPreparation) Delete() error {
	var transactionID string
	err := RDB.QueryRow(`SELECT transaction_id FROM employee_salary_preparations WHERE id = ?`, p.ID).Scan(&transactionID)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if transactionID != "" {
		bt := BalanceTransaction{ID: transactionID}
		if err := bt.Delete(); err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
	}
	res, err := DB.Exec(`DELETE FROM employee_salary_preparations WHERE id = ?`, p.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func getOrCreateBalanceWithTx(tx *sql.Tx, entityType, entityID, entityName, currencyID string) (*Balance, error) {
	now := DateNow()
	id := uuid.Must(uuid.NewV7()).String()
	if _, err := tx.Exec(`INSERT OR IGNORE INTO balances (`+balanceColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		id, entityType, entityID, entityName, currencyID, 0, 0, 0, now, now); err != nil {
		return nil, err
	}
	var b Balance
	if err := tx.QueryRow(`SELECT `+balanceColumns+` FROM balances WHERE entity_type = ? AND entity_id = ? AND currency_id = ?`,
		entityType, entityID, currencyID).Scan(&b.ID, &b.EntityType, &b.EntityID, &b.EntityName, &b.CurrencyID, &b.Amount, &b.TotalIn, &b.TotalOut, &b.CreatedAt, &b.UpdatedAt); err != nil {
		return nil, err
	}
	return &b, nil
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
