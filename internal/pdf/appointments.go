package pdf

import (
	"clinic-api/internal/database/store"
	"fmt"
	"strings"
	"time"

	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/orientation"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/props"
)

// appointmentCancelled is the store status a cancelled appointment carries.
const appointmentCancelled = "Cancelled"

// GenerateAppointments writes a PDF listing the day's appointments and returns
// its path. rooms maps room ids to names for display. Cancelled appointments are
// listed too, flagged in the status column with the cancellation reason in the
// notes column.
func GenerateAppointments(apts store.AppointmentList, rooms map[string]string, date string) (string, error) {
	if _, err := time.Parse(store.DateFormat, date); err != nil {
		return "", fmt.Errorf("date %q is not a calendar day", date)
	}
	cfg := config.NewBuilder().
		WithMaxGridSize(100).
		WithOrientation(orientation.Horizontal).
		WithLeftMargin(8).
		WithRightMargin(8).
		WithTopMargin(8).
		WithBottomMargin(10).
		WithPageNumber(props.PageNumber{
			Pattern: "Zeal Clinic — Appointments      {current} / {total}",
			Place:   props.RightBottom,
			Size:    8,
			Color:   clrMutedFg,
		}).
		Build()
	m := maroto.New(cfg)

	// Masthead + column header repeat on every page.
	hdr := &rowBuf{}
	masthead(hdr, 100, "Appointments", []string{"Date: " + clinicDate(store.Date(date))})
	titleBar(hdr, 100, "Schedule")
	hdr.AddRow(8,
		headerCell(9, "Date", align.Left),
		headerCell(9, "Room", align.Left),
		headerCell(11, "Time", align.Left),
		headerCell(15, "Patient", align.Left),
		headerCell(9, "Balance", align.Right),
		headerCell(22, "Procedures", align.Left),
		headerCell(16, "Notes", align.Left),
		headerCell(9, "Status", align.Left),
	)
	if err := m.RegisterHeader(hdr.rows...); err != nil {
		return "", err
	}

	// AddAutoRow sizes each row to its tallest cell, so a long procedure list
	// wraps onto extra lines and pushes the row taller instead of bleeding into
	// the row below. Every cell in the row is drawn to that height, keeping the
	// grid borders aligned.
	for _, a := range apts {
		m.AddAutoRow(
			bodyCell(9, beirutDate(a.StartTime), align.Left),
			bodyCell(9, rooms[a.RoomID], align.Left),
			bodyCell(11, appointmentTimeRange(a), align.Left),
			bodyCell(15, a.PatientName, align.Left),
			bodyCell(9, appointmentBalance(a), align.Right),
			bodyCell(22, appointmentProcedures(a), align.Left),
			bodyCell(16, appointmentNotes(a), align.Left),
			appointmentStatusCell(9, a),
		)
	}

	return save(m, "appointments-"+date)
}

// appointmentTimeRange formats the appointment window in clinic-local time as
// "HH:mm - HH:mm".
func appointmentTimeRange(a store.Appointment) string {
	return beirutHour(a.StartTime) + " - " + beirutHour(a.EndTime)
}

// beirutTime parses a stored appointment timestamp into clinic-local (Beirut)
// time. Storage is UTC, so every datetime is converted: zoned RFC3339 by its
// offset, and the naive formats validation.DateTime also accepts as UTC. A
// bare date has no time component and is kept as the clinic-local calendar day.
func beirutTime(d store.Date) (time.Time, bool) {
	s := string(d)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.In(store.ClinicLocation()), true
	}
	for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t.In(store.ClinicLocation()), true
		}
	}
	if t, err := time.ParseInLocation("2006-01-02", s, store.ClinicLocation()); err == nil {
		return t, true
	}
	return time.Time{}, false
}

func beirutDate(d store.Date) string {
	if t, ok := beirutTime(d); ok {
		return t.Format("02/01/2006")
	}
	return ""
}

func beirutHour(d store.Date) string {
	if t, ok := beirutTime(d); ok {
		return t.Format("3:04 PM")
	}
	return ""
}

// appointmentBalance renders the patient's balance, negative when they owe the
// clinic. Blank when the list method that loaded the appointment didn't hydrate
// it, so the column degrades to empty rather than a misleading 0.00.
func appointmentBalance(a store.Appointment) string {
	if a.PatientBalance == nil {
		return ""
	}
	return money(*a.PatientBalance)
}

// appointmentStatusCell renders the status column. The sheet lists cancelled
// appointments alongside the ones that stand, so a cancelled row is printed in
// the negative colour to keep it from reading as a booking to honour.
func appointmentStatusCell(size int, a store.Appointment) core.Col {
	if a.Status == appointmentCancelled {
		return bodyCellColor(size, a.Status, align.Left, clrNegative)
	}
	return bodyCell(size, a.Status, align.Left)
}

// appointmentNotes shows the appointment's notes, or the cancellation reason
// when it was cancelled, since that's the note explaining the row.
func appointmentNotes(a store.Appointment) string {
	if a.Status == appointmentCancelled {
		return strings.TrimSpace(a.CancelNotes)
	}
	return strings.TrimSpace(a.Notes)
}

func appointmentProcedures(a store.Appointment) string {
	names := make([]string, 0, len(a.AppointmentProcedures))
	for _, ap := range a.AppointmentProcedures {
		if n := strings.TrimSpace(ap.ProcedureName); n != "" {
			names = append(names, n)
		}
	}
	return strings.Join(names, "; ")
}
