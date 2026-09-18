package store

import (
	"errors"
	"testing"

	"clinic-api/internal/validation"

	"github.com/google/uuid"
)

// makeEmployee inserts a minimal employee for use in HR tests.
func makeEmployee(t *testing.T, first, last string) Employee {
	t.Helper()
	e := Employee{
		FirstName:      first,
		LastName:       last,
		Role:           "doctor",
		Contact:        "+961 1 111 111",
		EmploymentType: "Full-time",
	}
	if err := e.Create(); err != nil {
		t.Fatalf("makeEmployee: %v", err)
	}
	return e
}

func shift(start, end string) ScheduleShift {
	return ScheduleShift{StartTime: start, EndTime: end}
}

// setSchedule saves one weekday's complete shift set, effective from startDate.
func setSchedule(t *testing.T, employeeID string, dayOfWeek int, startDate Date, shifts ...ScheduleShift) []EmployeeSchedule {
	t.Helper()
	v := EmployeeScheduleVersion{
		EmployeeID: employeeID,
		DayOfWeek:  dayOfWeek,
		StartDate:  startDate,
		Shifts:     shifts,
	}
	rows, err := v.Save()
	if err != nil {
		t.Fatalf("save schedule day %d from %s: %v", dayOfWeek, startDate, err)
	}
	return rows
}

// findScheduleDay returns the projected day for a given workDate, or fails the
// test if it isn't present.
func findScheduleDay(t *testing.T, days []EmployeeScheduleDay, workDate Date) EmployeeScheduleDay {
	t.Helper()
	for _, d := range days {
		if d.WorkDate == workDate {
			return d
		}
	}
	t.Fatalf("no schedule day for %s", workDate)
	return EmployeeScheduleDay{}
}

// projectWeek projects the week 2026-11-01..2026-11-07, which contains the
// Wednesday (dayOfWeek=3) the shift tests below are built around.
func projectWeek(t *testing.T, employeeID string) []EmployeeScheduleDay {
	t.Helper()
	days, err := EmployeeScheduleForRange(employeeID, "2026-11-01", "2026-11-07")
	if err != nil {
		t.Fatalf("project schedule: %v", err)
	}
	return days
}

func assertShifts(t *testing.T, day EmployeeScheduleDay, want ...ScheduleShift) {
	t.Helper()
	if len(day.Shifts) != len(want) {
		t.Fatalf("got %d shifts, want %d: %+v", len(day.Shifts), len(want), day.Shifts)
	}
	for i, w := range want {
		got := day.Shifts[i]
		if got.StartTime != w.StartTime || got.EndTime != w.EndTime {
			t.Fatalf("shift %d = %s-%s, want %s-%s", i, got.StartTime, got.EndTime, w.StartTime, w.EndTime)
		}
	}
}

// multiple shifts per weekday

func TestEmployeeScheduleVersion_MultipleShiftsInOneDay(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Ivy", "Ashgrove")

	// Split day: mornings and late afternoons, with the break as the gap.
	setSchedule(t, emp.ID, 3, "2026-01-01", shift("09:00", "13:00"), shift("15:00", "18:00"))

	day := findScheduleDay(t, projectWeek(t, emp.ID), "2026-11-04")
	if day.IsOff {
		t.Fatalf("split day should be a working day")
	}
	assertShifts(t, day, shift("09:00", "13:00"), shift("15:00", "18:00"))
	if !approxEqual(day.Hours, 7) {
		t.Fatalf("expected 4 + 3 = 7 hours, got %v", day.Hours)
	}

	templates, err := SchedulesForWeek(emp.ID, "2026-11-01", "2026-11-07")
	if err != nil {
		t.Fatalf("templates: %v", err)
	}
	if len(templates) != 2 {
		t.Fatalf("expected both shifts as templates, got %d: %+v", len(templates), templates)
	}
	for _, row := range templates {
		if !row.IsActive {
			t.Fatalf("every shift of the current version should be active: %+v", row)
		}
	}
}

func TestEmployeeScheduleVersion_ShiftsAreSortedRegardlessOfInputOrder(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Jad", "Brightwater")

	setSchedule(t, emp.ID, 3, "2026-01-01", shift("15:00", "18:00"), shift("09:00", "13:00"))

	day := findScheduleDay(t, projectWeek(t, emp.ID), "2026-11-04")
	assertShifts(t, day, shift("09:00", "13:00"), shift("15:00", "18:00"))
}

func TestEmployeeScheduleVersion_RejectsOverlappingShifts(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Karim", "Coldbrook")

	v := EmployeeScheduleVersion{
		EmployeeID: emp.ID,
		DayOfWeek:  3,
		StartDate:  "2026-01-01",
		Shifts:     []ScheduleShift{shift("09:00", "14:00"), shift("13:00", "18:00")},
	}
	var ve validation.Errors
	if err := func() error { _, err := v.Save(); return err }(); !errors.As(err, &ve) {
		t.Fatalf("expected a validation error for overlapping shifts, got %v", err)
	}
	if ve["shifts"] == "" {
		t.Fatalf("expected a shifts error, got %v", ve)
	}
}

func TestEmployeeScheduleVersion_RejectsOvernightAndInvalidTimes(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Lara", "Dunmore")

	cases := []struct {
		name  string
		shift ScheduleShift
	}{
		{"overnight", shift("22:00", "02:00")},
		{"zero length", shift("09:00", "09:00")},
		{"unparseable", shift("9am", "5pm")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := EmployeeScheduleVersion{
				EmployeeID: emp.ID,
				DayOfWeek:  3,
				StartDate:  "2026-01-01",
				Shifts:     []ScheduleShift{tc.shift},
			}
			var ve validation.Errors
			if _, err := v.Save(); !errors.As(err, &ve) {
				t.Fatalf("expected a validation error, got %v", err)
			}
		})
	}
}

func TestEmployeeScheduleVersion_EmptyShiftsTurnsWeekdayOff(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Maya", "Elmsley")

	setSchedule(t, emp.ID, 3, "2026-01-01", shift("09:00", "17:00"))
	setSchedule(t, emp.ID, 3, "2026-11-04")

	days, err := EmployeeScheduleForRange(emp.ID, "2026-10-25", "2026-11-07")
	if err != nil {
		t.Fatalf("project schedule: %v", err)
	}
	if before := findScheduleDay(t, days, "2026-10-28"); before.IsOff {
		t.Fatalf("the week before the change should still be worked")
	}
	after := findScheduleDay(t, days, "2026-11-04")
	if !after.IsOff || after.OffReason != "no-schedule" {
		t.Fatalf("expected an unscheduled day off, got isOff=%v reason=%q", after.IsOff, after.OffReason)
	}
}

// version chain

func TestEmployeeScheduleVersion_SupersedesPreviousVersion(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Nadia", "Fairweather")

	setSchedule(t, emp.ID, 3, "2026-01-01", shift("09:00", "17:00"))
	setSchedule(t, emp.ID, 3, "2026-11-04", shift("10:00", "13:00"), shift("14:00", "16:00"))

	days, err := EmployeeScheduleForRange(emp.ID, "2026-10-25", "2026-11-07")
	if err != nil {
		t.Fatalf("project schedule: %v", err)
	}
	assertShifts(t, findScheduleDay(t, days, "2026-10-28"), shift("09:00", "17:00"))
	assertShifts(t, findScheduleDay(t, days, "2026-11-04"), shift("10:00", "13:00"), shift("14:00", "16:00"))
}

func TestEmployeeScheduleVersion_SameStartDateReplaces(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Omar", "Glenholm")

	setSchedule(t, emp.ID, 3, "2026-01-01", shift("09:00", "13:00"), shift("15:00", "18:00"))
	setSchedule(t, emp.ID, 3, "2026-01-01", shift("10:00", "16:00"))

	assertShifts(t, findScheduleDay(t, projectWeek(t, emp.ID), "2026-11-04"), shift("10:00", "16:00"))

	// The replaced shifts are gone, not kept as zero-width superseded rows.
	templates, err := SchedulesForWeek(emp.ID, "2026-11-01", "2026-11-07")
	if err != nil {
		t.Fatalf("templates: %v", err)
	}
	if len(templates) != 1 {
		t.Fatalf("expected 1 template row after replacing the version, got %d: %+v", len(templates), templates)
	}
}

func TestEmployeeScheduleVersion_SlotsInBetweenExistingVersions(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Pia", "Hollowell")

	setSchedule(t, emp.ID, 3, "2026-01-01", shift("09:00", "17:00"))
	setSchedule(t, emp.ID, 3, "2026-12-01", shift("08:00", "12:00"))
	// Backdated between the two: it must bound itself at the later version
	// rather than swallow it.
	setSchedule(t, emp.ID, 3, "2026-06-01", shift("13:00", "18:00"))

	days, err := EmployeeScheduleForRange(emp.ID, "2026-05-01", "2026-12-31")
	if err != nil {
		t.Fatalf("project schedule: %v", err)
	}
	assertShifts(t, findScheduleDay(t, days, "2026-05-06"), shift("09:00", "17:00"))
	assertShifts(t, findScheduleDay(t, days, "2026-11-04"), shift("13:00", "18:00"))
	assertShifts(t, findScheduleDay(t, days, "2026-12-09"), shift("08:00", "12:00"))
}

func TestEmployeeScheduleVersion_DefaultsStartDateToToday(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Uma", "Ironside")

	// Today is read on both sides of the save, so a run that crosses the
	// clinic's midnight still has a fixed expectation.
	before := ClinicToday()
	rows := setSchedule(t, emp.ID, 3, "", shift("09:00", "17:00"))
	after := ClinicToday()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row written, got %d", len(rows))
	}
	if got := rows[0].StartDate; got != before && got != after {
		t.Fatalf("expected start date to default to today (%s), got %q", after, got)
	}
	if !rows[0].EndDate.IsZero() {
		t.Fatalf("a version with no successor should be left open, got end date %q", rows[0].EndDate)
	}
	if !rows[0].IsActive {
		t.Fatalf("a version with no successor should be active")
	}
}

func TestEmployeeScheduleVersion_NormalizesTimestampStartDate(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Rami", "Kestrel")

	// A timestamp start date normalizes to its calendar day so it does not
	// sort after the plain work date and skip the version's first day.
	setSchedule(t, emp.ID, 3, "2026-11-04T10:30:00Z", shift("09:00", "17:00"))

	day := findScheduleDay(t, projectWeek(t, emp.ID), "2026-11-04")
	assertShifts(t, day, shift("09:00", "17:00"))
}

func TestEmployeeSchedule_DeleteRemovesWholeVersion(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Sara", "Larkspur")

	rows := setSchedule(t, emp.ID, 3, "2026-01-01", shift("09:00", "13:00"), shift("15:00", "18:00"))
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows written, got %d", len(rows))
	}

	sa := EmployeeSchedule{ID: rows[0].ID}
	if err := sa.Delete(); err != nil {
		t.Fatalf("delete version: %v", err)
	}

	templates, err := SchedulesForWeek(emp.ID, "2026-11-01", "2026-11-07")
	if err != nil {
		t.Fatalf("templates: %v", err)
	}
	if len(templates) != 0 {
		t.Fatalf("expected the whole version gone, got %d rows: %+v", len(templates), templates)
	}
	if day := findScheduleDay(t, projectWeek(t, emp.ID), "2026-11-04"); !day.IsOff {
		t.Fatalf("deleting the only version should leave the day uncovered")
	}
}

// Legacy schedule fixtures
//
// Legacy single-row-per-weekday schedule rows, inserted directly, bypassing
// Save, so the resolver is tested against the stored shape it must still read.

func insertLegacySchedule(t *testing.T, employeeID string, dayOfWeek int, startTime, endTime string, startDate, endDate Date, isActive bool) string {
	t.Helper()
	id := uuid.Must(uuid.NewV7()).String()
	now := DateNow()
	if _, err := DB.Exec(`INSERT INTO employee_schedules (`+employeeScheduleColumns+`)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		id, employeeID, dayOfWeek, startTime, endTime, startDate, endDate, BoolToInt(isActive), now, now); err != nil {
		t.Fatalf("insert legacy schedule: %v", err)
	}
	return id
}

func TestLegacySchedule_SingleActiveRowProjectsUnchanged(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Legacy", "Single")

	insertLegacySchedule(t, emp.ID, 3, "09:00", "17:00", "2026-01-01", "", true)

	day := findScheduleDay(t, projectWeek(t, emp.ID), "2026-11-04")
	assertShifts(t, day, shift("09:00", "17:00"))
	if !approxEqual(day.Hours, 8) {
		t.Fatalf("expected 8 hours, got %v", day.Hours)
	}
}

func TestLegacySchedule_SupersededChainProjectsUnchanged(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Legacy", "Chain")

	// Stored legacy shape: the earlier row closed at the later row's start
	// date, the later row open.
	insertLegacySchedule(t, emp.ID, 3, "09:00", "17:00", "2026-01-01", "2026-11-04", false)
	insertLegacySchedule(t, emp.ID, 3, "10:00", "14:00", "2026-11-04", "", true)

	days, err := EmployeeScheduleForRange(emp.ID, "2026-10-25", "2026-11-07")
	if err != nil {
		t.Fatalf("project schedule: %v", err)
	}
	assertShifts(t, findScheduleDay(t, days, "2026-10-28"), shift("09:00", "17:00"))
	assertShifts(t, findScheduleDay(t, days, "2026-11-04"), shift("10:00", "14:00"))
}

func TestLegacySchedule_ZeroWidthSupersededRowIsIgnored(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Legacy", "ZeroWidth")

	// Saving a weekday twice on the same day left the first row with
	// start_date == end_date. It covers no dates, so it must not surface as a
	// second shift now that a weekday can legitimately hold several.
	insertLegacySchedule(t, emp.ID, 3, "08:00", "12:00", "2026-01-01", "2026-01-01", false)
	insertLegacySchedule(t, emp.ID, 3, "09:00", "17:00", "2026-01-01", "", true)

	day := findScheduleDay(t, projectWeek(t, emp.ID), "2026-11-04")
	assertShifts(t, day, shift("09:00", "17:00"))
	if !approxEqual(day.Hours, 8) {
		t.Fatalf("a dead row must not add hours, got %v", day.Hours)
	}
}

func TestLegacySchedule_EmptyStartDateStillApplies(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Legacy", "NoStartDate")

	// start_date defaults to '' in the schema; such a row has always been read
	// as "in force from the beginning".
	insertLegacySchedule(t, emp.ID, 3, "09:00", "17:00", "", "", true)

	assertShifts(t, findScheduleDay(t, projectWeek(t, emp.ID), "2026-11-04"), shift("09:00", "17:00"))
}

func TestLegacySchedule_SplitDaySaveKeepsHistory(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Legacy", "Upgrade")

	// The upgrade path: an existing single-shift weekday, then the first
	// multi-shift save against it.
	insertLegacySchedule(t, emp.ID, 3, "09:00", "17:00", "2026-01-01", "", true)
	setSchedule(t, emp.ID, 3, "2026-11-04", shift("09:00", "13:00"), shift("15:00", "18:00"))

	days, err := EmployeeScheduleForRange(emp.ID, "2026-10-25", "2026-11-07")
	if err != nil {
		t.Fatalf("project schedule: %v", err)
	}
	// The old version keeps covering the dates it used to.
	assertShifts(t, findScheduleDay(t, days, "2026-10-28"), shift("09:00", "17:00"))
	assertShifts(t, findScheduleDay(t, days, "2026-11-04"), shift("09:00", "13:00"), shift("15:00", "18:00"))

	// The legacy row is still there as an editable superseded version.
	templates, err := SchedulesForWeek(emp.ID, "2026-10-25", "2026-11-07")
	if err != nil {
		t.Fatalf("templates: %v", err)
	}
	if len(templates) != 3 {
		t.Fatalf("expected the legacy row plus 2 new shifts, got %d: %+v", len(templates), templates)
	}
}

func TestLegacySchedule_TimestampStartDateAppliesOnItsFirstDay(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Legacy", "Timestamp")

	// The legacy row below starts at a timestamp, not a plain date.
	// A timestamp start_date resolves on the calendar day it names so it
	// does not sort after the plain work date and skip the version's first day.
	insertLegacySchedule(t, emp.ID, 3, "09:00", "17:00", "2026-11-04T08:30:00Z", "", true)

	assertShifts(t, findScheduleDay(t, projectWeek(t, emp.ID), "2026-11-04"), shift("09:00", "17:00"))
}

func TestLegacySchedule_DeleteRemovesOnlyThatRow(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Legacy", "Delete")

	// A version-scoped delete on single-row legacy versions removes exactly
	// that row.
	id := insertLegacySchedule(t, emp.ID, 3, "09:00", "17:00", "2026-01-01", "2026-11-04", false)
	insertLegacySchedule(t, emp.ID, 3, "10:00", "14:00", "2026-11-04", "", true)

	sa := EmployeeSchedule{ID: id}
	if err := sa.Delete(); err != nil {
		t.Fatalf("delete legacy version: %v", err)
	}

	days, err := EmployeeScheduleForRange(emp.ID, "2026-10-25", "2026-11-07")
	if err != nil {
		t.Fatalf("project schedule: %v", err)
	}
	if before := findScheduleDay(t, days, "2026-10-28"); !before.IsOff {
		t.Fatalf("the deleted version's dates should be uncovered, got %+v", before.Shifts)
	}
	assertShifts(t, findScheduleDay(t, days, "2026-11-04"), shift("10:00", "14:00"))
}

// timeoff

func TestEmployeeSchedule_PartialTimeoffClipsRightEdge(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Alice", "Smith")

	// Wednesday 2026-11-04 is dayOfWeek=3 in Go's time.Weekday (Sun=0).
	setSchedule(t, emp.ID, 3, "2026-01-01", shift("09:00", "18:00"))

	off := EmployeeScheduleChange{
		EmployeeID: emp.ID,
		Type:       "timeoff",
		StartDate:  "2026-11-04",
		EndDate:    "2026-11-04",
		StartTime:  "13:00",
		EndTime:    "18:00",
		Status:     "accepted",
	}
	if err := off.Create(); err != nil {
		t.Fatalf("create timeoff: %v", err)
	}

	day := findScheduleDay(t, projectWeek(t, emp.ID), "2026-11-04")
	if day.IsOff {
		t.Fatalf("expected day to remain partially worked, got isOff=true")
	}
	assertShifts(t, day, shift("09:00", "13:00"))
	if !approxEqual(day.Hours, 4) {
		t.Fatalf("expected 4 hours after partial timeoff, got %v", day.Hours)
	}
}

func TestEmployeeSchedule_PartialTimeoffClipsLeftEdge(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Bob", "Jones")

	setSchedule(t, emp.ID, 3, "2026-01-01", shift("09:00", "18:00"))

	// Timeoff in the morning: employee comes in late.
	off := EmployeeScheduleChange{
		EmployeeID: emp.ID,
		Type:       "timeoff",
		StartDate:  "2026-11-04",
		EndDate:    "2026-11-04",
		StartTime:  "09:00",
		EndTime:    "12:00",
		Status:     "accepted",
	}
	if err := off.Create(); err != nil {
		t.Fatalf("create timeoff: %v", err)
	}

	day := findScheduleDay(t, projectWeek(t, emp.ID), "2026-11-04")
	if day.IsOff {
		t.Fatalf("expected partial day, got fully off")
	}
	assertShifts(t, day, shift("12:00", "18:00"))
}

func TestEmployeeSchedule_PartialTimeoffCarvesMiddle(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Carol", "Doe")

	setSchedule(t, emp.ID, 3, "2026-01-01", shift("09:00", "18:00"))

	// Two-hour off block in the middle leaves two work segments.
	off := EmployeeScheduleChange{
		EmployeeID: emp.ID,
		Type:       "timeoff",
		StartDate:  "2026-11-04",
		EndDate:    "2026-11-04",
		StartTime:  "12:00",
		EndTime:    "14:00",
		Status:     "accepted",
	}
	if err := off.Create(); err != nil {
		t.Fatalf("create timeoff: %v", err)
	}

	day := findScheduleDay(t, projectWeek(t, emp.ID), "2026-11-04")
	assertShifts(t, day, shift("09:00", "12:00"), shift("14:00", "18:00"))
	if !approxEqual(day.Hours, 7) {
		t.Fatalf("expected 7 hours, got %v", day.Hours)
	}
}

func TestEmployeeSchedule_TimeoffAcrossSplitDay(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Tarek", "Marchbank")

	setSchedule(t, emp.ID, 3, "2026-01-01", shift("09:00", "13:00"), shift("15:00", "18:00"))

	// Straddles the break: clips the tail of the first shift and the head of
	// the second.
	off := EmployeeScheduleChange{
		EmployeeID: emp.ID,
		Type:       "timeoff",
		StartDate:  "2026-11-04",
		EndDate:    "2026-11-04",
		StartTime:  "12:00",
		EndTime:    "16:00",
		Status:     "accepted",
	}
	if err := off.Create(); err != nil {
		t.Fatalf("create timeoff: %v", err)
	}

	day := findScheduleDay(t, projectWeek(t, emp.ID), "2026-11-04")
	assertShifts(t, day, shift("09:00", "12:00"), shift("16:00", "18:00"))
	if !approxEqual(day.Hours, 5) {
		t.Fatalf("expected 3 + 2 = 5 hours, got %v", day.Hours)
	}
}

func TestEmployeeSchedule_PendingTimeoffDoesNotClip(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Dan", "Lee")

	setSchedule(t, emp.ID, 3, "2026-01-01", shift("09:00", "18:00"))

	off := EmployeeScheduleChange{
		EmployeeID: emp.ID,
		Type:       "timeoff",
		StartDate:  "2026-11-04",
		EndDate:    "2026-11-04",
		Status:     "pending",
	}
	if err := off.Create(); err != nil {
		t.Fatalf("create timeoff: %v", err)
	}

	day := findScheduleDay(t, projectWeek(t, emp.ID), "2026-11-04")
	if day.IsOff {
		t.Fatalf("pending timeoff should not affect projection")
	}
	if !approxEqual(day.Hours, 9) {
		t.Fatalf("expected full 9-hour shift, got %v", day.Hours)
	}
}

// overtime

func TestEmployeeSchedule_OvertimeAddsHours(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Eve", "Park")

	setSchedule(t, emp.ID, 3, "2026-01-01", shift("09:00", "18:00"))

	ot := EmployeeScheduleChange{
		EmployeeID: emp.ID,
		Type:       "overtime",
		StartDate:  "2026-11-04",
		EndDate:    "2026-11-04",
		StartTime:  "18:00",
		EndTime:    "21:00",
		Status:     "accepted",
	}
	if err := ot.Create(); err != nil {
		t.Fatalf("create overtime: %v", err)
	}

	day := findScheduleDay(t, projectWeek(t, emp.ID), "2026-11-04")
	if !approxEqual(day.Hours, 12) {
		t.Fatalf("expected 9 regular + 3 overtime = 12 hours, got %v", day.Hours)
	}
	if !approxEqual(day.OvertimeHours, 3) {
		t.Fatalf("expected 3 overtime hours, got %v", day.OvertimeHours)
	}
	assertShifts(t, day, shift("09:00", "18:00"), shift("18:00", "21:00"))
}

func TestEmployeeSchedule_OvertimeOverlappingShiftIsNotDoubleCounted(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Ziad", "Oakhurst")

	setSchedule(t, emp.ID, 3, "2026-01-01", shift("09:00", "13:00"))

	// Logged from noon, but noon to 13:00 is already scheduled work.
	ot := EmployeeScheduleChange{
		EmployeeID: emp.ID,
		Type:       "overtime",
		StartDate:  "2026-11-04",
		EndDate:    "2026-11-04",
		StartTime:  "12:00",
		EndTime:    "15:00",
		Status:     "accepted",
	}
	if err := ot.Create(); err != nil {
		t.Fatalf("create overtime: %v", err)
	}

	day := findScheduleDay(t, projectWeek(t, emp.ID), "2026-11-04")
	assertShifts(t, day, shift("09:00", "13:00"), shift("13:00", "15:00"))
	if !approxEqual(day.Hours, 6) {
		t.Fatalf("expected 6 hours actually worked, got %v", day.Hours)
	}
	if !approxEqual(day.OvertimeHours, 2) {
		t.Fatalf("expected only 13:00-15:00 to count as overtime, got %v", day.OvertimeHours)
	}
}

func TestEmployeeSchedule_OverlappingOvertimeWindowsMerge(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Wael", "Pemberly")

	setSchedule(t, emp.ID, 3, "2026-01-01", shift("09:00", "17:00"))

	for _, window := range [][2]string{{"17:00", "20:00"}, {"19:00", "22:00"}} {
		ot := EmployeeScheduleChange{
			EmployeeID: emp.ID,
			Type:       "overtime",
			StartDate:  "2026-11-04",
			EndDate:    "2026-11-04",
			StartTime:  window[0],
			EndTime:    window[1],
			Status:     "accepted",
		}
		if err := ot.Create(); err != nil {
			t.Fatalf("create overtime %v: %v", window, err)
		}
	}

	day := findScheduleDay(t, projectWeek(t, emp.ID), "2026-11-04")
	assertShifts(t, day, shift("09:00", "17:00"), shift("17:00", "22:00"))
	if !approxEqual(day.OvertimeHours, 5) {
		t.Fatalf("expected 17:00-22:00 = 5 overtime hours, got %v", day.OvertimeHours)
	}
}

func TestEmployeeSchedule_OvertimeAppliesToEveryDayInRange(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Faye", "Ravenscroft")

	// Schedule Mon-Fri (dayOfWeek 1-5).
	for day := 1; day <= 5; day++ {
		setSchedule(t, emp.ID, day, "2026-01-01", shift("09:00", "17:00"))
	}

	// Three days of evening overtime: Mon 2026-11-02, Tue 2026-11-03, Wed 2026-11-04.
	ot := EmployeeScheduleChange{
		EmployeeID: emp.ID,
		Type:       "overtime",
		StartDate:  "2026-11-02",
		EndDate:    "2026-11-04",
		StartTime:  "17:00",
		EndTime:    "20:00",
		Status:     "accepted",
	}
	if err := ot.Create(); err != nil {
		t.Fatalf("create overtime: %v", err)
	}

	days := projectWeek(t, emp.ID)
	for _, d := range []Date{"2026-11-02", "2026-11-03", "2026-11-04"} {
		day := findScheduleDay(t, days, d)
		if !approxEqual(day.OvertimeHours, 3) {
			t.Fatalf("%s: expected 3 overtime hours, got %v", d, day.OvertimeHours)
		}
		if !approxEqual(day.Hours, 11) {
			t.Fatalf("%s: expected 8 regular + 3 overtime = 11 hours, got %v", d, day.Hours)
		}
	}

	// Thursday is in the regular schedule but outside the overtime range.
	thu := findScheduleDay(t, days, "2026-11-05")
	if !approxEqual(thu.OvertimeHours, 0) {
		t.Fatalf("Thu expected no overtime, got %v", thu.OvertimeHours)
	}
	if !approxEqual(thu.Hours, 8) {
		t.Fatalf("Thu expected 8 regular hours only, got %v", thu.Hours)
	}
}

func TestEmployeeSchedule_TimeoffClipsOvertime(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Gus", "Stonebridge")

	setSchedule(t, emp.ID, 3, "2026-01-01", shift("09:00", "17:00"))

	ot := EmployeeScheduleChange{
		EmployeeID: emp.ID,
		Type:       "overtime",
		StartDate:  "2026-11-04",
		EndDate:    "2026-11-04",
		StartTime:  "17:00",
		EndTime:    "21:00",
		Status:     "accepted",
	}
	if err := ot.Create(); err != nil {
		t.Fatalf("create overtime: %v", err)
	}

	off := EmployeeScheduleChange{
		EmployeeID: emp.ID,
		Type:       "timeoff",
		StartDate:  "2026-11-04",
		EndDate:    "2026-11-04",
		StartTime:  "19:00",
		EndTime:    "21:00",
		Status:     "accepted",
	}
	if err := off.Create(); err != nil {
		t.Fatalf("create timeoff: %v", err)
	}

	day := findScheduleDay(t, projectWeek(t, emp.ID), "2026-11-04")
	if !approxEqual(day.OvertimeHours, 2) {
		t.Fatalf("expected 2 overtime hours (4 - 2 clipped by timeoff), got %v", day.OvertimeHours)
	}
	if !approxEqual(day.Hours, 10) {
		t.Fatalf("expected 8 regular + 2 overtime = 10, got %v", day.Hours)
	}
}

func TestEmployeeSchedule_HolidayDoesNotCancelOvertime(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Hala", "Thornfield")

	setSchedule(t, emp.ID, 3, "2026-01-01", shift("09:00", "17:00"))

	h := Holiday{
		Name:      "Public Holiday",
		StartDate: "2026-11-04",
		EndDate:   "2026-11-04",
	}
	if err := h.Create(); err != nil {
		t.Fatalf("create holiday: %v", err)
	}

	ot := EmployeeScheduleChange{
		EmployeeID: emp.ID,
		Type:       "overtime",
		StartDate:  "2026-11-04",
		EndDate:    "2026-11-04",
		StartTime:  "10:00",
		EndTime:    "14:00",
		Status:     "accepted",
	}
	if err := ot.Create(); err != nil {
		t.Fatalf("create overtime: %v", err)
	}

	day := findScheduleDay(t, projectWeek(t, emp.ID), "2026-11-04")
	if day.IsOff {
		t.Fatalf("holiday day with overtime should not be marked off")
	}
	// No regular shift survives the holiday, so nothing clips the window.
	if !approxEqual(day.Hours, 4) || !approxEqual(day.OvertimeHours, 4) {
		t.Fatalf("expected 4 total = 4 overtime on holiday, got hours=%v overtime=%v", day.Hours, day.OvertimeHours)
	}
}
