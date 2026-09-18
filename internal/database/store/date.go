package store

import (
	"database/sql/driver"
	"fmt"
	"sync"
	"time"
	// Embed the IANA tz database so Asia/Beirut resolves on minimal builds
	// (notably Windows containers without zoneinfo).
	_ "time/tzdata"
)

const (
	DateTimeFormat = time.RFC3339
	DateFormat     = "2006-01-02"
)

var (
	tzMu sync.RWMutex
	// clinicTimezone is the clinic's IANA timezone, set once at startup from
	// the config (SetClinicTimezone). All storage and API I/O remain UTC; this
	// is consulted only when the backend has to anchor "now / today / this
	// week / this month" reasoning on its own (no user input available to
	// derive the intended calendar day from).
	clinicTimezone = "Asia/Beirut"
	clinicLocation = func() *time.Location {
		loc, err := time.LoadLocation(clinicTimezone)
		if err != nil {
			return time.UTC
		}
		return loc
	}()
)

// SetClinicTimezone configures the clinic's IANA timezone and updates the
// cached location. An unknown zone is refused and leaves the current one.
func SetClinicTimezone(tz string) error {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return fmt.Errorf("load timezone %q: %w", tz, err)
	}
	tzMu.Lock()
	clinicTimezone = tz
	clinicLocation = loc
	tzMu.Unlock()
	return nil
}

// ClinicTimezoneName returns the configured IANA zone name.
func ClinicTimezoneName() string {
	tzMu.RLock()
	defer tzMu.RUnlock()
	return clinicTimezone
}

type Date string

// ClinicLocation returns the clinic's *time.Location, falling back to UTC if
// the tz database could not be loaded.
func ClinicLocation() *time.Location {
	tzMu.RLock()
	loc := clinicLocation
	tzMu.RUnlock()
	return loc
}

// ClinicNow returns the current wall-clock time in the clinic's timezone.
func ClinicNow() time.Time { return time.Now().In(ClinicLocation()) }

// ClinicToday returns today's clinic-local calendar date as YYYY-MM-DD.
func ClinicToday() Date {
	return Date(ClinicNow().Format(DateFormat))
}

// dayStart is the first instant of the calendar day (y, m, d) in loc. The day
// may be out of range (d+1, m+1, d-n): time.Date normalizes it like AddDate.
// When midnight falls into a DST gap the first instant of the day is later
// than 00:00 (Beirut: 01:00 on the last Sunday of March), which time.Date
// resolves as well.
func dayStart(y int, m time.Month, d int, loc *time.Location) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

// ClinicDayBounds returns the [start, end) UTC RFC3339 instants of the
// clinic-local calendar day containing t. The end is the first instant of the
// next calendar day, so the day of a DST switch is 23 or 25 hours long and
// never overlaps its neighbours.
func ClinicDayBounds(t time.Time) (Date, Date) {
	return ClinicDayBoundsIn(t, ClinicLocation())
}

// ClinicDayBoundsIn is ClinicDayBounds on the calendar of loc.
func ClinicDayBoundsIn(t time.Time, loc *time.Location) (Date, Date) {
	y, m, d := t.In(loc).Date()
	return DateFrom(dayStart(y, m, d, loc)), DateFrom(dayStart(y, m, d+1, loc))
}

// ClinicTodayBounds is the [start, end) UTC instants of today in the clinic's
// timezone.
func ClinicTodayBounds() (Date, Date) { return ClinicDayBounds(time.Now()) }

// ClinicWeekBounds returns the [start, end) UTC instants of the Sunday-Saturday
// week containing t, anchored in the clinic's timezone: from the first instant
// of that Sunday to the first instant of the next one.
func ClinicWeekBounds(t time.Time) (Date, Date) {
	return ClinicWeekBoundsIn(t, ClinicLocation())
}

// ClinicWeekBoundsIn is ClinicWeekBounds on the calendar of loc.
func ClinicWeekBoundsIn(t time.Time, loc *time.Location) (Date, Date) {
	local := t.In(loc)
	y, m, d := local.Date()
	sunday := d - int(local.Weekday())
	return DateFrom(dayStart(y, m, sunday, loc)), DateFrom(dayStart(y, m, sunday+7, loc))
}

// ClinicMonthBounds returns the [start, end) UTC instants of the calendar
// month containing t, anchored in the clinic's timezone.
func ClinicMonthBounds(t time.Time) (Date, Date) {
	return ClinicMonthBoundsIn(t, ClinicLocation())
}

// ClinicMonthBoundsIn is ClinicMonthBounds on the calendar of loc.
func ClinicMonthBoundsIn(t time.Time, loc *time.Location) (Date, Date) {
	y, m, _ := t.In(loc).Date()
	return DateFrom(dayStart(y, m, 1, loc)), DateFrom(dayStart(y, m+1, 1, loc))
}

// ClinicRangeLastNDays returns the [start, end) UTC instants of a window
// ending at the end of today (clinic) and reaching n calendar days back. Used
// to build default report ranges so the user's "last 30 days" matches their
// clinic calendar, not UTC.
func ClinicRangeLastNDays(n int) (Date, Date) {
	return ClinicRangeLastNDaysIn(time.Now(), ClinicLocation(), n)
}

// ClinicRangeLastNDaysIn is ClinicRangeLastNDays for the day of now on the
// calendar of loc.
func ClinicRangeLastNDaysIn(now time.Time, loc *time.Location, n int) (Date, Date) {
	y, m, d := now.In(loc).Date()
	return DateFrom(dayStart(y, m, d-n, loc)), DateFrom(dayStart(y, m, d+1, loc))
}

func DateNow() Date {
	return Date(time.Now().UTC().Format(DateTimeFormat))
}

// DateToday returns today's clinic-local calendar date as YYYY-MM-DD.
func DateToday() Date {
	return DateTodayIn(time.Now(), ClinicLocation())
}

// DateTodayIn returns the calendar date of now on the calendar of loc.
func DateTodayIn(now time.Time, loc *time.Location) Date {
	return Date(now.In(loc).Format(DateFormat))
}

func DateFrom(t time.Time) Date {
	return Date(t.UTC().Format(DateTimeFormat))
}

// DateOffsetDays returns today's clinic-local calendar date shifted by n
// calendar days as a YYYY-MM-DD Date. Negative n is in the past.
func DateOffsetDays(n int) Date {
	return DateOffsetDaysIn(time.Now(), ClinicLocation(), n)
}

// DateOffsetDaysIn returns the calendar date of now on the calendar of loc,
// shifted by n calendar days.
func DateOffsetDaysIn(now time.Time, loc *time.Location, n int) Date {
	return Date(now.In(loc).AddDate(0, 0, n).Format(DateFormat))
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

// isBareDate reports whether s is a YYYY-MM-DD date.
func isBareDate(s string) bool {
	if len(s) != 10 {
		return false
	}
	_, err := time.Parse(DateFormat, s)
	return err == nil
}

// PreviousPeriod returns the window of the same calendar span that ends where
// the half-open window [from, to) begins: whole calendar months give the
// previous months (April's previous period is March, however many days each
// has), anything else the same number of calendar days, so a window that
// crosses a DST switch shifts by whole clinic days, not by its duration.
// RFC3339 instants are read on the clinic calendar; bare YYYY-MM-DD dates are
// UTC days, as in RangeStart and RangeEnd. The result is returned as RFC3339
// UTC instants (which pass through RangeStart/RangeEnd unchanged). Returns
// empty strings if the bounds cannot be parsed.
func PreviousPeriod(from, to string) (string, string) {
	return PreviousPeriodIn(from, to, ClinicLocation())
}

// PreviousPeriodIn is PreviousPeriod with instants read on the calendar of loc.
func PreviousPeriodIn(from, to string, loc *time.Location) (string, string) {
	s, err1 := time.Parse(DateTimeFormat, RangeStart(from))
	e, err2 := time.Parse(DateTimeFormat, RangeEnd(to))
	if err1 != nil || err2 != nil || !e.After(s) {
		return "", ""
	}
	if isBareDate(from) && isBareDate(to) {
		loc = time.UTC
	}
	prev := shiftBackBySpan(s.In(loc), e.In(loc))
	return string(DateFrom(prev)), string(DateFrom(s))
}

// shiftBackBySpan returns the start of the window that has the calendar span
// of [s, e) and ends at s, on the calendar of s's location.
func shiftBackBySpan(s, e time.Time) time.Time {
	loc := s.Location()
	sy, sm, sd := s.Date()
	ey, em, ed := e.Date()
	days := calendarDaysBetween(s, e)
	if isDayStart(s) && isDayStart(e) {
		if sd == 1 && ed == 1 {
			months := (ey-sy)*12 + int(em-sm)
			return dayStart(sy, sm-time.Month(months), 1, loc)
		}
		return dayStart(sy, sm, sd-days, loc)
	}
	// Partial days: shift by whole calendar days keeping the wall clock, then
	// by the remaining duration.
	sameWall := func(d int) time.Time {
		return time.Date(sy, sm, sd+d, s.Hour(), s.Minute(), s.Second(), s.Nanosecond(), loc)
	}
	if sameWall(days).After(e) {
		days--
	}
	rest := e.Sub(sameWall(days))
	return sameWall(-days).Add(-rest)
}

// isDayStart reports whether t is the first instant of its calendar day.
func isDayStart(t time.Time) bool {
	y, m, d := t.Date()
	return t.Equal(dayStart(y, m, d, t.Location()))
}

// calendarDaysBetween counts the calendar days from the date of s to the date
// of e, both read in their own location.
func calendarDaysBetween(s, e time.Time) int {
	sy, sm, sd := s.Date()
	ey, em, ed := e.Date()
	return int(dayStart(ey, em, ed, time.UTC).Sub(dayStart(sy, sm, sd, time.UTC)).Hours() / 24)
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
