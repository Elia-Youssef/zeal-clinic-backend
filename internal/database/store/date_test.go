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
	a := Date("2024-01-01")               // date-only
	b := Date("2024-01-02T00:00:00Z")     // RFC3339
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
