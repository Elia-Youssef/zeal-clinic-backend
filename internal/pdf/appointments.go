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
	"github.com/johnfercher/maroto/v2/pkg/props"
)

// GenerateAppointments writes a PDF listing the day's appointments and returns
// its path. rooms maps room ids to names for display.
func GenerateAppointments(apts store.AppointmentList, rooms map[string]string, date string) (string, error) {
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
		headerCell(11, "Date", align.Left),
		headerCell(11, "Room", align.Left),
		headerCell(11, "Time", align.Left),
		headerCell(24, "Patient", align.Left),
		headerCell(43, "Procedures", align.Left),
	)
	if err := m.RegisterHeader(hdr.rows...); err != nil {
		return "", err
	}

	for _, a := range apts {
		m.AddRow(7,
			bodyCell(11, beirutDate(a.StartTime), align.Left),
			bodyCell(11, rooms[a.RoomID], align.Left),
			bodyCell(11, appointmentTimeRange(a), align.Left),
			bodyCell(24, a.PatientName, align.Left),
			bodyCell(43, appointmentProcedures(a), align.Left),
		)
	}

	return save(m, fmt.Sprintf("appointments-%s", date))
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

func appointmentProcedures(a store.Appointment) string {
	names := make([]string, 0, len(a.AppointmentProcedures))
	for _, ap := range a.AppointmentProcedures {
		if n := strings.TrimSpace(ap.ProcedureName); n != "" {
			names = append(names, n)
		}
	}
	return strings.Join(names, "; ")
}
