package store

import (
	"testing"
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

func TestEmployeeSchedule_PartialTimeoffClipsRightEdge(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Alice", "Smith")

	// Wednesday 2026-11-04 is dayOfWeek=3 in Go's time.Weekday (Sun=0).
	sa := EmployeeSchedule{
		EmployeeID: emp.ID,
		DayOfWeek:  3,
		StartTime:  "09:00",
		EndTime:    "18:00",
		StartDate:  "2026-01-01",
	}
	if err := sa.Create(); err != nil {
		t.Fatalf("create employee_schedules: %v", err)
	}

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

	days, err := EmployeeScheduleForRange(emp.ID, "2026-11-01", "2026-11-07")
	if err != nil {
		t.Fatalf("project schedule: %v", err)
	}

	day := findScheduleDay(t, days, "2026-11-04")
	if day.IsOff {
		t.Fatalf("expected day to remain partially worked, got isOff=true")
	}
	if len(day.Shifts) != 1 {
		t.Fatalf("expected 1 remaining shift, got %d: %+v", len(day.Shifts), day.Shifts)
	}
	got := day.Shifts[0]
	if got.StartTime != "09:00" || got.EndTime != "13:00" {
		t.Fatalf("expected 09:00-13:00 after subtracting 13:00-18:00 timeoff, got %s-%s", got.StartTime, got.EndTime)
	}
	if !approxEqual(day.Hours, 4) {
		t.Fatalf("expected 4 hours after partial timeoff, got %v", day.Hours)
	}
}

func TestEmployeeSchedule_PartialTimeoffClipsLeftEdge(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Bob", "Jones")

	sa := EmployeeSchedule{
		EmployeeID: emp.ID,
		DayOfWeek:  3,
		StartTime:  "09:00",
		EndTime:    "18:00",
		StartDate:  "2026-01-01",
	}
	if err := sa.Create(); err != nil {
		t.Fatalf("create employee_schedules: %v", err)
	}

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

	days, err := EmployeeScheduleForRange(emp.ID, "2026-11-01", "2026-11-07")
	if err != nil {
		t.Fatalf("project schedule: %v", err)
	}

	day := findScheduleDay(t, days, "2026-11-04")
	if day.IsOff {
		t.Fatalf("expected partial day, got fully off")
	}
	if len(day.Shifts) != 1 || day.Shifts[0].StartTime != "12:00" || day.Shifts[0].EndTime != "18:00" {
		t.Fatalf("expected 12:00-18:00 after morning timeoff, got %+v", day.Shifts)
	}
}

func TestEmployeeSchedule_PartialTimeoffCarvesMiddle(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Carol", "Doe")

	sa := EmployeeSchedule{
		EmployeeID: emp.ID,
		DayOfWeek:  3,
		StartTime:  "09:00",
		EndTime:    "18:00",
		StartDate:  "2026-01-01",
	}
	if err := sa.Create(); err != nil {
		t.Fatalf("create employee_schedules: %v", err)
	}

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

	days, err := EmployeeScheduleForRange(emp.ID, "2026-11-01", "2026-11-07")
	if err != nil {
		t.Fatalf("project schedule: %v", err)
	}

	day := findScheduleDay(t, days, "2026-11-04")
	if len(day.Shifts) != 2 {
		t.Fatalf("expected 2 shift segments, got %d: %+v", len(day.Shifts), day.Shifts)
	}
	if day.Shifts[0].StartTime != "09:00" || day.Shifts[0].EndTime != "12:00" {
		t.Fatalf("first segment wrong: %+v", day.Shifts[0])
	}
	if day.Shifts[1].StartTime != "14:00" || day.Shifts[1].EndTime != "18:00" {
		t.Fatalf("second segment wrong: %+v", day.Shifts[1])
	}
	if !approxEqual(day.Hours, 7) {
		t.Fatalf("expected 7 hours, got %v", day.Hours)
	}
}

func TestEmployeeSchedule_PendingTimeoffDoesNotClip(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Dan", "Lee")

	sa := EmployeeSchedule{
		EmployeeID: emp.ID,
		DayOfWeek:  3,
		StartTime:  "09:00",
		EndTime:    "18:00",
		StartDate:  "2026-01-01",
	}
	if err := sa.Create(); err != nil {
		t.Fatalf("create employee_schedules: %v", err)
	}

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

	days, err := EmployeeScheduleForRange(emp.ID, "2026-11-01", "2026-11-07")
	if err != nil {
		t.Fatalf("project schedule: %v", err)
	}

	day := findScheduleDay(t, days, "2026-11-04")
	if day.IsOff {
		t.Fatalf("pending timeoff should not affect projection")
	}
	if !approxEqual(day.Hours, 9) {
		t.Fatalf("expected full 9-hour shift, got %v", day.Hours)
	}
}

func TestEmployeeSchedule_OvertimeAddsHours(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Eve", "Park")

	sa := EmployeeSchedule{
		EmployeeID: emp.ID,
		DayOfWeek:  3,
		StartTime:  "09:00",
		EndTime:    "18:00",
		StartDate:  "2026-01-01",
	}
	if err := sa.Create(); err != nil {
		t.Fatalf("create employee_schedules: %v", err)
	}

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

	days, err := EmployeeScheduleForRange(emp.ID, "2026-11-01", "2026-11-07")
	if err != nil {
		t.Fatalf("project schedule: %v", err)
	}

	day := findScheduleDay(t, days, "2026-11-04")
	if !approxEqual(day.Hours, 12) {
		t.Fatalf("expected 9 regular + 3 overtime = 12 hours, got %v", day.Hours)
	}
	if !approxEqual(day.OvertimeHours, 3) {
		t.Fatalf("expected 3 overtime hours, got %v", day.OvertimeHours)
	}
	if len(day.Shifts) != 2 {
		t.Fatalf("expected 2 shifts (regular + overtime), got %d: %+v", len(day.Shifts), day.Shifts)
	}
}

func TestEmployeeSchedule_OvertimeAppliesToEveryDayInRange(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Faye", "Khoury")

	// Schedule Mon-Fri (dayOfWeek 1-5).
	for day := 1; day <= 5; day++ {
		sa := EmployeeSchedule{
			EmployeeID: emp.ID,
			DayOfWeek:  day,
			StartTime:  "09:00",
			EndTime:    "17:00",
			StartDate:  "2026-01-01",
		}
		if err := sa.Create(); err != nil {
			t.Fatalf("create employee_schedules day %d: %v", day, err)
		}
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

	days, err := EmployeeScheduleForRange(emp.ID, "2026-11-01", "2026-11-07")
	if err != nil {
		t.Fatalf("project schedule: %v", err)
	}

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
	emp := makeEmployee(t, "Gus", "Rahme")

	sa := EmployeeSchedule{
		EmployeeID: emp.ID,
		DayOfWeek:  3,
		StartTime:  "09:00",
		EndTime:    "17:00",
		StartDate:  "2026-01-01",
	}
	if err := sa.Create(); err != nil {
		t.Fatalf("create employee_schedules: %v", err)
	}

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

	days, err := EmployeeScheduleForRange(emp.ID, "2026-11-01", "2026-11-07")
	if err != nil {
		t.Fatalf("project schedule: %v", err)
	}

	day := findScheduleDay(t, days, "2026-11-04")
	if !approxEqual(day.OvertimeHours, 2) {
		t.Fatalf("expected 2 overtime hours (4 - 2 clipped by timeoff), got %v", day.OvertimeHours)
	}
	if !approxEqual(day.Hours, 10) {
		t.Fatalf("expected 8 regular + 2 overtime = 10, got %v", day.Hours)
	}
}

func TestEmployeeSchedule_HolidayDoesNotCancelOvertime(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Hala", "Saad")

	sa := EmployeeSchedule{
		EmployeeID: emp.ID,
		DayOfWeek:  3,
		StartTime:  "09:00",
		EndTime:    "17:00",
		StartDate:  "2026-01-01",
	}
	if err := sa.Create(); err != nil {
		t.Fatalf("create employee_schedules: %v", err)
	}

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

	days, err := EmployeeScheduleForRange(emp.ID, "2026-11-01", "2026-11-07")
	if err != nil {
		t.Fatalf("project schedule: %v", err)
	}

	day := findScheduleDay(t, days, "2026-11-04")
	if day.IsOff {
		t.Fatalf("holiday day with overtime should not be marked off")
	}
	if !approxEqual(day.Hours, 4) || !approxEqual(day.OvertimeHours, 4) {
		t.Fatalf("expected 4 total = 4 overtime on holiday, got hours=%v overtime=%v", day.Hours, day.OvertimeHours)
	}
}
