package store

import (
	"database/sql/driver"
	"fmt"
	"time"
	// Embed the IANA tz database so Asia/Beirut resolves on minimal builds
	// (notably Windows containers without zoneinfo).
	_ "time/tzdata"
)

const (
	DateTimeFormat = time.RFC3339
	DateFormat     = "2006-01-02"
	// ClinicTimezone is the clinic's IANA timezone. All storage and API I/O
	// remain UTC; this is consulted only when the backend has to anchor
	// "now / today / this week / this month" reasoning on its own (no user
	// input available to derive the intended calendar day from).
	ClinicTimezone = "Asia/Beirut"
)

type Date string

var clinicLocation = func() *time.Location {
	loc, err := time.LoadLocation(ClinicTimezone)
	if err != nil {
		return time.UTC
	}
	return loc
}()

// ClinicLocation returns the clinic's *time.Location, falling back to UTC if
// the tz database could not be loaded.
func ClinicLocation() *time.Location { return clinicLocation }

// ClinicNow returns the current wall-clock time in the clinic's timezone.
func ClinicNow() time.Time { return time.Now().In(clinicLocation) }

// ClinicToday returns today's clinic-local calendar date as YYYY-MM-DD.
func ClinicToday() Date {
	return Date(ClinicNow().Format(DateFormat))
}

// ClinicDayBounds returns the [start, end) UTC RFC3339 instants of the
// clinic-local calendar day containing t.
func ClinicDayBounds(t time.Time) (Date, Date) {
	local := t.In(clinicLocation)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, clinicLocation)
	end := start.AddDate(0, 0, 1)
	return DateFrom(start), DateFrom(end)
}

// ClinicTodayBounds is the [start, end) UTC instants of today in the clinic's
// timezone.
func ClinicTodayBounds() (Date, Date) { return ClinicDayBounds(ClinicNow()) }

// ClinicWeekBounds returns the [start, end) UTC instants of the Sunday-Saturday
// week containing t, anchored in the clinic's timezone.
func ClinicWeekBounds(t time.Time) (Date, Date) {
	local := t.In(clinicLocation)
	start := time.Date(local.Year(), local.Month(), local.Day()-int(local.Weekday()), 0, 0, 0, 0, clinicLocation)
	end := start.AddDate(0, 0, 7)
	return DateFrom(start), DateFrom(end)
}

// ClinicMonthBounds returns the [start, end) UTC instants of the calendar
// month containing t, anchored in the clinic's timezone.
func ClinicMonthBounds(t time.Time) (Date, Date) {
	local := t.In(clinicLocation)
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, clinicLocation)
	end := start.AddDate(0, 1, 0)
	return DateFrom(start), DateFrom(end)
}

// ClinicRangeLastNDays returns the [start, end) UTC instants of a window
// ending at the end of today (clinic) and reaching n days back. Used to
// build default report ranges so the user's "last 30 days" matches their
// clinic calendar, not UTC.
func ClinicRangeLastNDays(n int) (Date, Date) {
	now := ClinicNow()
	_, end := ClinicDayBounds(now)
	start, _ := ClinicDayBounds(now.AddDate(0, 0, -n))
	return start, end
}

func DateNow() Date {
	return Date(time.Now().UTC().Format(DateTimeFormat))
}

func DateToday() Date {
	return Date(time.Now().UTC().Format(DateFormat))
}

func DateFrom(t time.Time) Date {
	return Date(t.UTC().Format(DateTimeFormat))
}

// DateOffsetDays returns today's UTC calendar date shifted by n days as a
// YYYY-MM-DD Date. Negative n is in the past. Use this anywhere you'd otherwise
// reach for time.Now().UTC().AddDate(0, 0, n).Format("2006-01-02").
func DateOffsetDays(n int) Date {
	return Date(time.Now().UTC().AddDate(0, 0, n).Format(DateFormat))
}

// RangeStart normalizes a range lower bound to an inclusive RFC3339 UTC
// instant for half-open `[from, to)` filtering. A bare YYYY-MM-DD is treated
// as UTC start-of-day; an RFC3339 input is passed through unchanged (the
// frontend is responsible for converting clinic-local boundaries to UTC).
func RangeStart(s string) string {
	if s == "" {
		return ""
	}
	if len(s) == 10 {
		if t, err := time.Parse(DateFormat, s); err == nil {
			return t.UTC().Format(DateTimeFormat)
		}
	}
	return s
}

// RangeEnd normalizes a range upper bound to an exclusive RFC3339 UTC instant
// for half-open `[from, to)` filtering. A bare YYYY-MM-DD is expanded to the
// next day's UTC start-of-day so the calendar day itself is included; an
// RFC3339 input is passed through unchanged and treated as already exclusive.
func RangeEnd(s string) string {
	if s == "" {
		return ""
	}
	if len(s) == 10 {
		if t, err := time.Parse(DateFormat, s); err == nil {
			return t.UTC().AddDate(0, 0, 1).Format(DateTimeFormat)
		}
	}
	return s
}

func (d Date) String() string {
	return string(d)
}

func (d Date) IsZero() bool {
	return d == ""
}

func (d Date) Time() (time.Time, error) {
	if d.IsZero() {
		return time.Time{}, nil
	}
	if t, err := time.Parse(DateTimeFormat, string(d)); err == nil {
		return t, nil
	}
	if t, err := time.Parse(DateFormat, string(d)); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("cannot parse date: %s", string(d))
}

// Before / After compare on calendar-day granularity. We deliberately do not
// use time.Time.Before here: a value like "2026-11-05T00:00:00+02:00" parses
// to a moment that is 2026-11-04 22:00 UTC, which would shift the day relative
// to "2026-11-05". DateOnly() preserves the YYYY-MM-DD prefix when the input
// already starts with one, so the comparison stays in the sender's intended
// calendar day.
func (d Date) Before(other Date) bool {
	return d.DateOnly() < other.DateOnly()
}

func (d Date) After(other Date) bool {
	return d.DateOnly() > other.DateOnly()
}

// WeekRange returns the Sunday–Saturday week containing d as YYYY-MM-DD strings.
// Falls back to the clinic-local current week if d is empty or unparseable.
func WeekRange(d Date) (Date, Date) {
	t, err := d.Time()
	if err != nil || t.IsZero() {
		t = ClinicNow()
	}
	start := t.AddDate(0, 0, -int(t.Weekday()))
	end := start.AddDate(0, 0, 6)
	return Date(start.Format(DateFormat)), Date(end.Format(DateFormat))
}

// MonthRange returns the first and last day of the calendar month containing d
// as YYYY-MM-DD strings. Falls back to the clinic-local current month if d is
// empty or invalid.
func MonthRange(d Date) (Date, Date) {
	t, err := d.Time()
	if err != nil || t.IsZero() {
		t = ClinicNow()
	}
	start := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
	end := start.AddDate(0, 1, -1)
	return Date(start.Format(DateFormat)), Date(end.Format(DateFormat))
}

func (d Date) DateOnly() string {
	s := string(d)
	// Preserve the sender's calendar day when the value already starts with a
	// YYYY-MM-DD prefix (covers "2026-11-05" and any ISO-8601 datetime). This
	// avoids shifting the day if the trailing time has a non-UTC offset.
	if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
		if _, err := time.Parse(DateFormat, s[:10]); err == nil {
			return s[:10]
		}
	}
	t, err := d.Time()
	if err != nil {
		return s
	}
	return t.Format(DateFormat)
}

func (d *Date) Scan(value any) error {
	if value == nil {
		*d = ""
		return nil
	}
	switch v := value.(type) {
	case string:
		*d = Date(v)
	case []byte:
		*d = Date(string(v))
	case time.Time:
		*d = DateFrom(v)
	default:
		return fmt.Errorf("cannot scan %T into Date", value)
	}
	return nil
}

func (d Date) Value() (driver.Value, error) {
	return string(d), nil
}
