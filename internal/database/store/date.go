package store

import (
	"database/sql/driver"
	"fmt"
	"time"
)

const (
	DateTimeFormat = time.RFC3339
	DateFormat     = "2006-01-02"
)

type Date string

func DateNow() Date {
	return Date(time.Now().Format(DateTimeFormat))
}

func DateToday() Date {
	return Date(time.Now().Format(DateFormat))
}

func DateFrom(t time.Time) Date {
	return Date(t.Format(DateTimeFormat))
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
// Falls back to the current week if d is empty or unparseable.
func WeekRange(d Date) (Date, Date) {
	t, err := d.Time()
	if err != nil || t.IsZero() {
		t = time.Now()
	}
	start := t.AddDate(0, 0, -int(t.Weekday()))
	end := start.AddDate(0, 0, 6)
	return Date(start.Format(DateFormat)), Date(end.Format(DateFormat))
}

// MonthRange returns the first and last day of the calendar month containing d
// as YYYY-MM-DD strings. Falls back to the current month if d is empty or invalid.
func MonthRange(d Date) (Date, Date) {
	t, err := d.Time()
	if err != nil || t.IsZero() {
		t = time.Now()
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
