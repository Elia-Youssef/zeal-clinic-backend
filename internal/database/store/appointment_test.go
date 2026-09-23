package store

import (
	"testing"
	"time"
)

// The weekly grid counts every appointment on the clinic day of its start:
// one booked at 00:30 clinic time counts on that day although it is stored on
// the previous UTC day, one booked at 00:30 on the Monday after the week is
// left out although its UTC day is the week's Sunday, and cancelled ones do
// not count.
func TestGetAppointmentCountPerRoom_ClinicDays(t *testing.T) {
	setupTestDB(t)
	patient := makePatient(t, "Grid", "Patient", "70123456")
	if _, err := DB.Exec(`INSERT INTO rooms (id, name, type) VALUES ('room-grid', 'Grid Room', 'General')`); err != nil {
		t.Fatal(err)
	}
	at := func(day string, hour, minute int) Date {
		t.Helper()
		d, err := time.ParseInLocation(DateFormat, day, ClinicLocation())
		if err != nil {
			t.Fatal(err)
		}
		return DateFrom(time.Date(d.Year(), d.Month(), d.Day(), hour, minute, 0, 0, ClinicLocation()))
	}
	book := func(id, day string, hour, minute int, status string) {
		t.Helper()
		start := at(day, hour, minute)
		end := at(day, hour, minute+30)
		if _, err := DB.Exec(`INSERT INTO appointments (id, patient_id, room_id, start_time, end_time, status)
			VALUES (?, ?, 'room-grid', ?, ?, ?)`, id, patient.ID, start, end, status); err != nil {
			t.Fatal(err)
		}
	}
	book("a1", "2025-03-31", 0, 30, "Scheduled") // stored on 30 March UTC
	book("a2", "2025-03-31", 10, 0, "Completed")
	book("a3", "2025-04-06", 23, 30, "Scheduled") // the week's Sunday, late
	book("a4", "2025-04-07", 0, 30, "Scheduled")  // the next Monday, stored on 6 April UTC
	book("a5", "2025-04-01", 9, 0, "Cancelled")
	book("a6", "2025-03-30", 23, 30, "Scheduled") // the Sunday before the week

	items, weekStart, weekEnd, err := GetAppointmentCountPerRoom("2025-03-31")
	if err != nil {
		t.Fatal(err)
	}
	if weekStart != "2025-03-31" || weekEnd != "2025-04-06" {
		t.Errorf("week = [%s, %s], want the Monday 2025-03-31 to the Sunday 2025-04-06", weekStart, weekEnd)
	}
	// The week's Sunday reads back to the Monday before it.
	if _, ws, we, err := GetAppointmentCountPerRoom("2025-04-06"); err != nil || ws != "2025-03-31" || we != "2025-04-06" {
		t.Errorf("week of the Sunday 2025-04-06 = [%s, %s], %v, want the same Monday week", ws, we, err)
	}
	var got map[string]int
	for _, it := range items {
		if it.RoomID == "room-grid" {
			got = it.Days
		}
	}
	want := map[string]int{"2025-03-31": 2, "2025-04-06": 1}
	if len(got) != len(want) {
		t.Fatalf("days = %v, want %v", got, want)
	}
	for day, n := range want {
		if got[day] != n {
			t.Errorf("%s = %d, want %d (all: %v)", day, got[day], n, got)
		}
	}
}
