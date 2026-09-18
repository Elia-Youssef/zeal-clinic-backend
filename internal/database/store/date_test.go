package store

import (
	"strings"
	"testing"
	"time"
)

// PreviousPeriod must return the equal-length window ending exactly where the
// current one begins (half-open, no gap or overlap).
func TestPreviousPeriod(t *testing.T) {
	// For a 30-day window in May, the previous window is the 30 days ending May 1.
	pf, pt := PreviousPeriod("2026-05-01", "2026-05-30")
	// RangeEnd("2026-05-30") is exclusive next-day (2026-05-31), so the window is
	// 30 days; previous ends at the current start.
	if pt != "2026-05-01T00:00:00Z" {
		t.Errorf("previous end = %q, want current start 2026-05-01T00:00:00Z", pt)
	}
	if pf != "2026-04-01T00:00:00Z" {
		t.Errorf("previous start = %q, want 2026-04-01T00:00:00Z", pf)
	}
}

func TestPreviousPeriod_BadInput(t *testing.T) {
	if pf, pt := PreviousPeriod("", ""); pf != "" || pt != "" {
		t.Errorf("empty bounds should yield empty, got (%q,%q)", pf, pt)
	}
}

// Whole calendar months shift by months, so March's previous period is
// February whatever their lengths; any other span shifts by its number of
// days. Bare dates are UTC days, instants are read on the clinic calendar.
func TestPreviousPeriod_CalendarSpans(t *testing.T) {
	cases := []struct {
		name, from, to, wantFrom, wantTo string
	}{
		{"a month", "2025-03-01", "2025-03-31", "2025-02-01T00:00:00Z", "2025-03-01T00:00:00Z"},
		{"two months", "2025-02-01", "2025-03-31", "2024-12-01T00:00:00Z", "2025-02-01T00:00:00Z"},
		{"31 days off the month bounds", "2025-03-15", "2025-04-14", "2025-02-12T00:00:00Z", "2025-03-15T00:00:00Z"},
		{"a single day", "2025-03-01", "2025-03-01", "2025-02-28T00:00:00Z", "2025-03-01T00:00:00Z"},
		// Clinic midnights of 1 March and 1 April 2026 (EET, then EEST).
		{"a clinic month", "2026-02-28T22:00:00Z", "2026-03-31T21:00:00Z", "2026-01-31T22:00:00Z", "2026-02-28T22:00:00Z"},
		// Partial days keep their duration.
		{"twelve hours", "2025-03-01T06:00:00Z", "2025-03-01T18:00:00Z", "2025-02-28T18:00:00Z", "2025-03-01T06:00:00Z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			from, to := PreviousPeriod(tc.from, tc.to)
			if from != tc.wantFrom || to != tc.wantTo {
				t.Errorf("PreviousPeriod(%s, %s) = [%s, %s), want [%s, %s)", tc.from, tc.to, from, to, tc.wantFrom, tc.wantTo)
			}
		})
	}
}

// The In variants take the clock and the calendar, so the clinic-local day
// can be checked around midnight and across the DST switch.
func TestDateHelpers_ClinicLocalDays(t *testing.T) {
	loc := ClinicLocation()
	// 21:30 UTC on 29 March 2026 is already 00:30 on 30 March in Beirut.
	now := mustInstant(t, "2026-03-29T21:30:00Z")
	if got := DateTodayIn(now, loc); got != "2026-03-30" {
		t.Errorf("DateTodayIn(clinic) = %s, want 2026-03-30", got)
	}
	if got := DateTodayIn(now, time.UTC); got != "2026-03-29" {
		t.Errorf("DateTodayIn(UTC) = %s, want 2026-03-29", got)
	}
	if got := DateOffsetDaysIn(now, loc, -1); got != "2026-03-29" {
		t.Errorf("DateOffsetDaysIn(-1) = %s, want 2026-03-29", got)
	}
	if got := DateOffsetDaysIn(now, loc, 2); got != "2026-04-01" {
		t.Errorf("DateOffsetDaysIn(2) = %s, want 2026-04-01", got)
	}
	// The last two clinic days: from the first instant of the 23-hour day
	// (01:00 EEST) to the end of 30 March.
	if s, e := ClinicRangeLastNDaysIn(now, loc, 1); s != "2026-03-28T22:00:00Z" || e != "2026-03-30T21:00:00Z" {
		t.Errorf("ClinicRangeLastNDaysIn(clinic, 1) = [%s, %s)", s, e)
	}
	if s, e := ClinicRangeLastNDaysIn(now, time.UTC, 1); s != "2026-03-28T00:00:00Z" || e != "2026-03-30T00:00:00Z" {
		t.Errorf("ClinicRangeLastNDaysIn(UTC, 1) = [%s, %s)", s, e)
	}
	if s, e := ClinicDayBoundsIn(now, time.UTC); s != "2026-03-29T00:00:00Z" || e != "2026-03-30T00:00:00Z" {
		t.Errorf("ClinicDayBoundsIn(UTC) = [%s, %s)", s, e)
	}
	if s, e := ClinicWeekBoundsIn(now, loc); s != "2026-03-28T22:00:00Z" || e != "2026-04-04T21:00:00Z" {
		t.Errorf("ClinicWeekBoundsIn(clinic) = [%s, %s)", s, e)
	}
	if s, e := ClinicMonthBoundsIn(now, loc); s != "2026-02-28T22:00:00Z" || e != "2026-03-31T21:00:00Z" {
		t.Errorf("ClinicMonthBoundsIn(clinic) = [%s, %s)", s, e)
	}
}

// newMetric computes the fractional change and guards divide-by-zero.
func TestNewMetric(t *testing.T) {
	if m := newMetric(120, 100); m.Change < 0.1999 || m.Change > 0.2001 {
		t.Errorf("change = %v, want ~0.2", m.Change)
	}
	if m := newMetric(50, 0); m.Change != 0 {
		t.Errorf("change with zero previous = %v, want 0", m.Change)
	}
}

// Time() is the core parser: empty is the zero value (no error), RFC3339 and
// date-only both parse, anything else errors.
func TestDate_TimeParsing(t *testing.T) {
	if got, err := Date("").Time(); err != nil || !got.IsZero() {
		t.Errorf(`Date("").Time() = (%v, %v), want (zero, nil)`, got, err)
	}
	if _, err := Date("2024-01-02T03:04:05Z").Time(); err != nil {
		t.Errorf("RFC3339 should parse: %v", err)
	}
	got, err := Date("2024-01-02").Time()
	if err != nil {
		t.Fatalf("date-only should parse: %v", err)
	}
	if got.Year() != 2024 || got.Month() != 1 || got.Day() != 2 {
		t.Errorf("date-only parsed wrong: %v", got)
	}
	if _, err := Date("not-a-date").Time(); err == nil {
		t.Error("garbage should error")
	}
}

// Before/After drive SQL-independent sorting. They must handle mixed formats and
// fall back to lexical comparison when a value can't be parsed.
func TestDate_Ordering(t *testing.T) {
	a := Date("2024-01-01")           // date-only
	b := Date("2024-01-02T00:00:00Z") // RFC3339
	if !a.Before(b) || !b.After(a) {
		t.Errorf("mixed-format comparison wrong: a<b should hold")
	}
	if a.Before(a) || a.After(a) {
		t.Error("a date is neither before nor after itself")
	}
	// Unparseable values fall back to lexical order.
	if !Date("aaa").Before(Date("bbb")) {
		t.Error("lexical fallback failed")
	}
}

// Scan/Value implement the sql driver interfaces; round-tripping must be lossless
// for strings, bytes, time.Time and nil.
func TestDate_ScanValueRoundTrip(t *testing.T) {
	var d Date
	if err := (&d).Scan("2024-01-01"); err != nil || d != "2024-01-01" {
		t.Errorf("Scan(string) = %q, %v", d, err)
	}
	if err := (&d).Scan([]byte("2024-02-02")); err != nil || d != "2024-02-02" {
		t.Errorf("Scan([]byte) = %q, %v", d, err)
	}
	if err := (&d).Scan(nil); err != nil || d != "" {
		t.Errorf("Scan(nil) should clear, got %q, %v", d, err)
	}
	if err := (&d).Scan(time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)); err != nil ||
		!strings.HasPrefix(string(d), "2024-06-01T12:00:00") {
		t.Errorf("Scan(time.Time) = %q, %v", d, err)
	}
	if err := (&d).Scan(42); err == nil {
		t.Error("Scan(int) should error")
	}

	v, err := Date("2024-01-01").Value()
	if err != nil || v != "2024-01-01" {
		t.Errorf("Value() = %v, %v", v, err)
	}
}

func TestDate_DateOnly(t *testing.T) {
	cases := map[string]string{
		"2024-05-17T10:30:00Z": "2024-05-17",
		"2024-05-17":           "2024-05-17",
		"garbage":              "garbage", // unparseable returns the raw input
	}
	for in, want := range cases {
		if got := Date(in).DateOnly(); got != want {
			t.Errorf("DateOnly(%q) = %q want %q", in, got, want)
		}
	}
}
