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

func TestEmployeeSchedule_PartialVacationClipsRightEdge(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Alice", "Smith")

	// Wednesday 2026-11-04 is dayOfWeek=3 in Go's time.Weekday (Sun=0).
	sa := ScheduleAvailability{
		EmployeeID: emp.ID,
		DayOfWeek:  3,
		StartTime:  "09:00",
		EndTime:    "18:00",
		StartDate:  "2026-01-01",
	}
	if err := sa.Create(); err != nil {
		t.Fatalf("create schedule_availability: %v", err)
	}

	vac := EmployeeVacation{
		EmployeeID: emp.ID,
		StartDate:  "2026-11-04",
		EndDate:    "2026-11-04",
		StartTime:  "13:00",
		EndTime:    "18:00",
		Status:     "accepted",
	}
	if err := vac.Create(); err != nil {
		t.Fatalf("create vacation: %v", err)
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
		t.Fatalf("expected 09:00-13:00 after subtracting 13:00-18:00 vacation, got %s-%s", got.StartTime, got.EndTime)
	}
	if !approxEqual(day.Hours, 4) {
		t.Fatalf("expected 4 hours after partial vacation, got %v", day.Hours)
	}
}

func TestEmployeeSchedule_PartialVacationClipsLeftEdge(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Bob", "Jones")

	sa := ScheduleAvailability{
		EmployeeID: emp.ID,
		DayOfWeek:  3,
		StartTime:  "09:00",
		EndTime:    "18:00",
		StartDate:  "2026-01-01",
	}
	if err := sa.Create(); err != nil {
		t.Fatalf("create schedule_availability: %v", err)
	}

	// Vacation in the morning: employee comes in late.
	vac := EmployeeVacation{
		EmployeeID: emp.ID,
		StartDate:  "2026-11-04",
		EndDate:    "2026-11-04",
		StartTime:  "09:00",
		EndTime:    "12:00",
		Status:     "accepted",
	}
	if err := vac.Create(); err != nil {
		t.Fatalf("create vacation: %v", err)
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
		t.Fatalf("expected 12:00-18:00 after morning vacation, got %+v", day.Shifts)
	}
}

func TestEmployeeSchedule_PartialVacationCarvesMiddle(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Carol", "Doe")

	sa := ScheduleAvailability{
		EmployeeID: emp.ID,
		DayOfWeek:  3,
		StartTime:  "09:00",
		EndTime:    "18:00",
		StartDate:  "2026-01-01",
	}
	if err := sa.Create(); err != nil {
		t.Fatalf("create schedule_availability: %v", err)
	}

	// Two-hour off block in the middle leaves two work segments.
	vac := EmployeeVacation{
		EmployeeID: emp.ID,
		StartDate:  "2026-11-04",
		EndDate:    "2026-11-04",
		StartTime:  "12:00",
		EndTime:    "14:00",
		Status:     "accepted",
	}
	if err := vac.Create(); err != nil {
		t.Fatalf("create vacation: %v", err)
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

func TestEmployeeSchedule_PendingVacationDoesNotClip(t *testing.T) {
	setupTestDB(t)
	emp := makeEmployee(t, "Dan", "Lee")

	sa := ScheduleAvailability{
		EmployeeID: emp.ID,
		DayOfWeek:  3,
		StartTime:  "09:00",
		EndTime:    "18:00",
		StartDate:  "2026-01-01",
	}
	if err := sa.Create(); err != nil {
		t.Fatalf("create schedule_availability: %v", err)
	}

	vac := EmployeeVacation{
		EmployeeID: emp.ID,
		StartDate:  "2026-11-04",
		EndDate:    "2026-11-04",
		Status:     "pending",
	}
	if err := vac.Create(); err != nil {
		t.Fatalf("create vacation: %v", err)
	}

	days, err := EmployeeScheduleForRange(emp.ID, "2026-11-01", "2026-11-07")
	if err != nil {
		t.Fatalf("project schedule: %v", err)
	}

	day := findScheduleDay(t, days, "2026-11-04")
	if day.IsOff {
		t.Fatalf("pending vacation should not affect projection")
	}
	if !approxEqual(day.Hours, 9) {
		t.Fatalf("expected full 9-hour shift, got %v", day.Hours)
	}
}
