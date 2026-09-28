package store

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/google/uuid"
)

const employeeScheduleColumnsNoId = `employee_id, day_of_week, start_time, end_time, start_date, end_date, is_active, created_at, updated_at`
const employeeScheduleColumns = `id, ` + employeeScheduleColumnsNoId

// One shift of one weekday. Rows sharing (employee_id, day_of_week, start_date)
// are a *version*: the complete set of shifts worked on that weekday from
// start_date until end_date (exclusive). A weekday can hold several shifts
// (09:00-13:00 then 15:00-18:00) as long as they don't overlap; the gaps between
// them are the breaks. Superseded versions keep is_active=0 and an end_date so
// historical dates still resolve. Versions are written whole, through
// EmployeeScheduleVersion.Save.
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

type EmployeeScheduleList []EmployeeSchedule

// A contiguous block of working time. Kind is "regular" or "overtime".
type ScheduleShift struct {
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
	Kind      string `json:"kind,omitempty"`
}

// EmployeeScheduleVersion is the full shift set for one weekday, effective from
// StartDate. Saving it replaces every shift on that weekday from that date on:
// the shifts sent are the shifts that exist. An empty Shifts list turns the
// weekday into a day off.
type EmployeeScheduleVersion struct {
	EmployeeID string          `json:"employeeId"`
	DayOfWeek  int             `json:"dayOfWeek"`
	StartDate  Date            `json:"startDate"`
	Shifts     []ScheduleShift `json:"shifts"`
}

// normalize trims StartDate to a calendar day and orders the shifts, so the
// overlap check below only has to compare each shift with the one before it.
// A timestamp start_date would compare wrong against the YYYY-MM-DD work dates
// the projection resolves against, so it is never stored as one.
func (v *EmployeeScheduleVersion) normalize() {
	// Checked before DateOnly, which renders an empty Date as "0001-01-01".
	if v.StartDate.IsZero() {
		v.StartDate = ClinicToday()
	} else {
		v.StartDate = Date(v.StartDate.DateOnly())
	}
	for i := range v.Shifts {
		v.Shifts[i].Kind = "regular"
	}
	sortShiftsByStart(v.Shifts)
}

func (v *EmployeeScheduleVersion) IsValid() error {
	e := make(validation.Errors)
	if msg := validation.Required(v.EmployeeID, "Employee ID"); msg != "" {
		e["employeeId"] = msg
	}
	if v.DayOfWeek < 0 || v.DayOfWeek > 6 {
		e["dayOfWeek"] = "Day of week must be between 0 and 6"
	}
	if msg := validation.Date(v.StartDate.DateOnly()); msg != "" {
		e["startDate"] = msg
	}

	// Shifts are sorted by normalize, so an overlap can only be with the
	// previous one.
	prevEnd := -1
	for i, s := range v.Shifts {
		startMin, okStart := minutesOfDay(s.StartTime)
		endMin, okEnd := minutesOfDay(s.EndTime)
		if !okStart || !okEnd {
			e["shifts"] = fmt.Sprintf("Shift %d has an invalid time (expected HH:MM)", i+1)
			break
		}
		// An overnight shift would have to span two weekday rows. The clinic
		// doesn't run them, so reject it rather than silently score 0 hours.
		if endMin <= startMin {
			e["shifts"] = fmt.Sprintf("Shift %d must end after it starts, on the same day", i+1)
			break
		}
		if startMin < prevEnd {
			e["shifts"] = fmt.Sprintf("Shift %d overlaps the one before it", i+1)
			break
		}
		prevEnd = endMin
	}

	if len(e) > 0 {
		return e
	}
	return nil
}

// Save writes Shifts as the version of this weekday in force from StartDate on.
// Any version already starting on StartDate is replaced, the version running at
// StartDate is closed there, and versions that start later keep their own
// windows, so a version can be slotted in between two existing ones without
// disturbing the chain. Returns the rows written, empty when the weekday is
// being turned into a day off.
func (v *EmployeeScheduleVersion) Save() ([]EmployeeSchedule, error) {
	shifts, err := v.save()
	return shifts, constraintError(err, "")
}

func (v *EmployeeScheduleVersion) save() ([]EmployeeSchedule, error) {
	v.normalize()
	if err := v.IsValid(); err != nil {
		return nil, err
	}

	tx, err := DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// The next version along bounds this one, so the chain stays contiguous
	// whether this save appends to the end or lands between two versions.
	var nextStart Date
	if err := tx.QueryRow(`SELECT MIN(start_date) FROM employee_schedules
		WHERE employee_id = ? AND day_of_week = ? AND substr(start_date, 1, 10) > ?`,
		v.EmployeeID, v.DayOfWeek, v.StartDate).Scan(&nextStart); err != nil {
		return nil, err
	}
	// A pre-existing row may hold a timestamp; the boundary this writes must not.
	// MIN is NULL when nothing follows, and DateOnly would turn that into a
	// real-looking "0001-01-01" end_date, so leave an empty value alone.
	if !nextStart.IsZero() {
		nextStart = Date(nextStart.DateOnly())
	}

	now := DateNow()

	// A version starting on the same date is being rewritten, not superseded:
	// closing it out would leave zero-width rows behind.
	if _, err := tx.Exec(`DELETE FROM employee_schedules
		WHERE employee_id = ? AND day_of_week = ? AND substr(start_date, 1, 10) = ?`,
		v.EmployeeID, v.DayOfWeek, v.StartDate); err != nil {
		return nil, err
	}

	// Close the version that was running when this one starts. Only versions
	// that started earlier are superseded; later ones are a separate chapter.
	if _, err := tx.Exec(`UPDATE employee_schedules
		SET is_active = 0, end_date = ?, updated_at = ?
		WHERE employee_id = ? AND day_of_week = ? AND substr(start_date, 1, 10) < ?
		  AND (end_date = '' OR substr(end_date, 1, 10) > ?)`,
		v.StartDate, now, v.EmployeeID, v.DayOfWeek, v.StartDate, v.StartDate); err != nil {
		return nil, err
	}

	// is_active marks the newest version of the weekday. Every shift in that
	// version is active at once; a version slotted in before an existing one is
	// already superseded the moment it is written.
	isActive := nextStart.IsZero()
	out := make([]EmployeeSchedule, 0, len(v.Shifts))
	for _, s := range v.Shifts {
		row := EmployeeSchedule{
			ID:         uuid.Must(uuid.NewV7()).String(),
			EmployeeID: v.EmployeeID,
			DayOfWeek:  v.DayOfWeek,
			StartTime:  s.StartTime,
			EndTime:    s.EndTime,
			StartDate:  v.StartDate,
			EndDate:    nextStart,
			IsActive:   isActive,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		if _, err := tx.Exec(`INSERT INTO employee_schedules (`+employeeScheduleColumns+`)
			VALUES (?,?,?,?,?,?,?,?,?,?)`,
			row.ID, row.EmployeeID, row.DayOfWeek, row.StartTime, row.EndTime,
			row.StartDate, row.EndDate, BoolToInt(row.IsActive), row.CreatedAt, row.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

// Delete removes the whole version the row belongs to: every shift sharing its
// (employee, weekday, start_date). The dates it covered are left uncovered on
// purpose: the versions around it keep their own boundaries.
func (sa *EmployeeSchedule) Delete() error {
	var current EmployeeSchedule
	if err := current.GetByID(sa.ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	res, err := DB.Exec(`DELETE FROM employee_schedules
		WHERE employee_id = ? AND day_of_week = ? AND start_date = ?`,
		current.EmployeeID, current.DayOfWeek, current.StartDate)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

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
			// A dropped row silently removes a shift from the schedule, so say so.
			log.Println("Error: EmployeeScheduleList.ScanRows skipping row:", err)
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

// SchedulesForWeek returns the employee_schedules rows in force at any point
// during the given week, superseded versions included: a week before a schedule
// change is projected from the version that was current then, so that version
// has to come back with it to stay editable. Several rows can share a
// (day_of_week, start_date); those are the shifts of one version. end_date is
// exclusive here, the same way shiftsInForce reads it. employeeID == "" returns
// every employee's.
func SchedulesForWeek(employeeID string, weekStart, weekEnd Date) ([]EmployeeSchedule, error) {
	where := ` WHERE substr(sa.start_date, 1, 10) <= ? AND (sa.end_date = '' OR substr(sa.end_date, 1, 10) > ?)`
	args := []any{weekEnd, weekStart}
	if employeeID != "" {
		where += ` AND sa.employee_id = ?`
		args = append(args, employeeID)
	}
	rows, err := RDB.Query(employeeScheduleSelect()+`
		FROM employee_schedules sa
		JOIN employees e ON e.id = sa.employee_id`+where+`
		ORDER BY e.first_name, sa.day_of_week, sa.start_date, sa.start_time`, args...)
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
		byWeekday, err := scheduleVersionsForRange(emp.id, from, to)
		if err != nil {
			return nil, err
		}
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

			template := shiftsInForce(byWeekday[int(d.Weekday())], workDate)
			_, isHoliday := holidays[workDate]

			var regularShifts []ScheduleShift
			if !isHoliday {
				regularShifts = template
			}

			var overtimeShifts []ScheduleShift
			for _, ch := range changes {
				if ch.Type != "overtime" || !changeAppliesToDay(ch, workDate) {
					continue
				}
				overtimeShifts = append(overtimeShifts, ScheduleShift{
					StartTime: ch.StartTime, EndTime: ch.EndTime, Kind: "overtime",
				})
			}
			// Two approved windows covering the same hour are one stretch of
			// work, not two.
			overtimeShifts = mergeShifts(overtimeShifts)
			// Overtime is the part of the window worked outside the scheduled
			// shift; the overlap is already counted as regular hours. On a
			// holiday there is no regular shift to clip against, so the whole
			// window stays overtime.
			for _, r := range regularShifts {
				rStart, okStart := minutesOfDay(r.StartTime)
				rEnd, okEnd := minutesOfDay(r.EndTime)
				if !okStart || !okEnd {
					continue
				}
				overtimeShifts = subtractInterval(overtimeShifts, rStart, rEnd)
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

			shifts := append(append([]ScheduleShift{}, regularShifts...), overtimeShifts...)
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
				case len(template) == 0:
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

// scheduleVersionsForRange loads every schedule row for the employee in force at
// some point in [from, to], bucketed by weekday. Loading the range in one query
// keeps the per-day resolution in memory, where the date comparisons run on
// calendar days rather than raw column text.
func scheduleVersionsForRange(employeeID string, from Date, to Date) (map[int][]EmployeeSchedule, error) {
	rows, err := RDB.Query(`SELECT day_of_week, start_time, end_time, start_date, end_date
		FROM employee_schedules
		WHERE employee_id = ?
		  AND substr(start_date, 1, 10) <= ?
		  AND (end_date = '' OR substr(end_date, 1, 10) > ?)
		ORDER BY day_of_week, start_date, start_time`, employeeID, to, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[int][]EmployeeSchedule{}
	for rows.Next() {
		var r EmployeeSchedule
		if err := rows.Scan(&r.DayOfWeek, &r.StartTime, &r.EndTime, &r.StartDate, &r.EndDate); err != nil {
			return nil, err
		}
		out[r.DayOfWeek] = append(out[r.DayOfWeek], r)
	}
	return out, rows.Err()
}

// shiftsInForce returns the shifts of the version in force on workDate: every
// row sharing the latest start_date that still covers the date. An empty result
// means the employee doesn't work that weekday. rows must all be for the one
// weekday.
func shiftsInForce(rows []EmployeeSchedule, workDate Date) []ScheduleShift {
	var version Date
	found := false
	for _, r := range rows {
		if !scheduleRowCovers(r, workDate) {
			continue
		}
		if !found || version.Before(r.StartDate) {
			version, found = r.StartDate, true
		}
	}
	if !found {
		return nil
	}

	out := []ScheduleShift{}
	for _, r := range rows {
		if !scheduleRowCovers(r, workDate) || r.StartDate.DateOnly() != version.DateOnly() {
			continue
		}
		out = append(out, ScheduleShift{StartTime: r.StartTime, EndTime: r.EndTime, Kind: "regular"})
	}
	sortShiftsByStart(out)
	return out
}

// scheduleRowCovers reports whether a version row is in force on workDate.
// end_date is exclusive: a version ends the day its successor starts.
func scheduleRowCovers(r EmployeeSchedule, workDate Date) bool {
	if r.StartDate.After(workDate) {
		return false
	}
	return r.EndDate.IsZero() || r.EndDate.After(workDate)
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

// mergeShifts folds overlapping or touching shifts into one, dropping any with
// unreadable or empty windows. Callers pass shifts of a single kind.
func mergeShifts(shifts []ScheduleShift) []ScheduleShift {
	sortShiftsByStart(shifts)
	out := make([]ScheduleShift, 0, len(shifts))
	for _, s := range shifts {
		start, okStart := minutesOfDay(s.StartTime)
		end, okEnd := minutesOfDay(s.EndTime)
		if !okStart || !okEnd || end <= start {
			continue
		}
		if len(out) > 0 {
			prev := &out[len(out)-1]
			if prevEnd, ok := minutesOfDay(prev.EndTime); ok && start <= prevEnd {
				if end > prevEnd {
					prev.EndTime = s.EndTime
				}
				continue
			}
		}
		out = append(out, s)
	}
	return out
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

// Typed readers for the map[string]any update payloads. Shared with the other
// HR models (holidays, schedule changes).

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
