package store

import (
	"testing"
	"time"
)

// Beirut switches at local midnight on the last Sunday of March (00:00 jumps
// to 01:00, so that day has 23 hours) and of October (00:00 falls back to
// 23:00 of the Saturday, so that Saturday has 25 hours). Each vector is the
// first instant on the new offset.
var beirutDSTVectors = []struct {
	at                     string
	lastBefore, firstAfter string
}{
	{"2026-03-28T22:00:00Z", "2026-03-28 23:59:59 +0200", "2026-03-29 01:00:00 +0300"},
	{"2026-10-24T21:00:00Z", "2026-10-24 23:59:59 +0300", "2026-10-24 23:00:00 +0200"},
	{"2027-03-27T22:00:00Z", "2027-03-27 23:59:59 +0200", "2027-03-28 01:00:00 +0300"},
	{"2027-10-30T21:00:00Z", "2027-10-30 23:59:59 +0300", "2027-10-30 23:00:00 +0200"},
}

// The vectors are checked against the embedded tz database first, so a
// tzdata change fails here instead of in the bounds tests below.
func TestClinicLocation_DSTVectors(t *testing.T) {
	const layout = "2006-01-02 15:04:05 -0700"
	for _, v := range beirutDSTVectors {
		at := mustInstant(t, v.at)
		if got := at.Add(-time.Second).In(ClinicLocation()).Format(layout); got != v.lastBefore {
			t.Errorf("%s minus 1s in clinic time = %s, want %s", v.at, got, v.lastBefore)
		}
		if got := at.In(ClinicLocation()).Format(layout); got != v.firstAfter {
			t.Errorf("%s in clinic time = %s, want %s", v.at, got, v.firstAfter)
		}
	}
}

func TestClinicDayBounds_AcrossDST(t *testing.T) {
	cases := []struct {
		day        string
		start, end string
		hours      int
	}{
		{"2026-03-27", "2026-03-26T22:00:00Z", "2026-03-27T22:00:00Z", 24},
		{"2026-03-28", "2026-03-27T22:00:00Z", "2026-03-28T22:00:00Z", 24},
		// The 23-hour day: midnight does not exist, the start normalizes to
		// 01:00 and the end is taken one calendar day later at 01:00, so the
		// window is 24 hours and reaches one hour into 30 March.
		{"2026-03-29", "2026-03-28T22:00:00Z", "2026-03-29T22:00:00Z", 24},
		{"2026-03-30", "2026-03-29T21:00:00Z", "2026-03-30T21:00:00Z", 24},
		{"2026-10-24", "2026-10-23T21:00:00Z", "2026-10-24T22:00:00Z", 25},
		{"2026-10-25", "2026-10-24T22:00:00Z", "2026-10-25T22:00:00Z", 24},
		{"2027-03-28", "2027-03-27T22:00:00Z", "2027-03-28T22:00:00Z", 24},
		{"2027-10-30", "2027-10-29T21:00:00Z", "2027-10-30T22:00:00Z", 25},
	}
	for _, tc := range cases {
		t.Run(tc.day, func(t *testing.T) {
			noon := clinicTime(t, tc.day, 12, 0)
			start, end := ClinicDayBounds(noon)
			if string(start) != tc.start || string(end) != tc.end {
				t.Fatalf("ClinicDayBounds(%s noon) = [%s, %s), want [%s, %s)", tc.day, start, end, tc.start, tc.end)
			}
			if h := hoursBetween(t, start, end); h != tc.hours {
				t.Errorf("window length = %dh, want %dh", h, tc.hours)
			}
			s2, e2 := ClinicDayBounds(mustInstant(t, tc.start))
			if s2 != start || e2 != end {
				t.Errorf("bounds from the first instant = [%s, %s), want the same window", s2, e2)
			}
		})
	}
}

// The windows of 29 and 30 March 2026 overlap: 00:00-00:59 on 30 March
// belongs to 30 March but also sits inside the 29 March window.
func TestClinicDayBounds_SpringForwardWindowsOverlap(t *testing.T) {
	inOverlap := mustInstant(t, "2026-03-29T21:30:00Z")
	start, end := ClinicDayBounds(inOverlap)
	if start != "2026-03-29T21:00:00Z" || end != "2026-03-30T21:00:00Z" {
		t.Fatalf("bounds of 30 March 00:30 = [%s, %s)", start, end)
	}
	_, prevEnd := ClinicDayBounds(clinicTime(t, "2026-03-29", 12, 0))
	if !mustInstant(t, string(prevEnd)).After(inOverlap) {
		t.Fatalf("29 March window ends at %s; expected it to still contain %s", prevEnd, inOverlap.Format(time.RFC3339))
	}
}

func TestClinicWeekBounds_AcrossDST(t *testing.T) {
	cases := []struct {
		day        string
		start, end string
		hours      int
	}{
		{"2026-03-25", "2026-03-21T22:00:00Z", "2026-03-28T22:00:00Z", 168},
		// Weeks starting on the 23-hour Sunday are 168 hours long and end an
		// hour into the next week, like the day bounds above.
		{"2026-03-29", "2026-03-28T22:00:00Z", "2026-04-04T22:00:00Z", 168},
		{"2026-04-01", "2026-03-28T22:00:00Z", "2026-04-04T22:00:00Z", 168},
		{"2026-04-05", "2026-04-04T21:00:00Z", "2026-04-11T21:00:00Z", 168},
		{"2026-10-24", "2026-10-17T21:00:00Z", "2026-10-24T22:00:00Z", 169},
		{"2026-10-25", "2026-10-24T22:00:00Z", "2026-10-31T22:00:00Z", 168},
		{"2027-10-30", "2027-10-23T21:00:00Z", "2027-10-30T22:00:00Z", 169},
	}
	for _, tc := range cases {
		t.Run(tc.day, func(t *testing.T) {
			start, end := ClinicWeekBounds(clinicTime(t, tc.day, 12, 0))
			if string(start) != tc.start || string(end) != tc.end {
				t.Fatalf("ClinicWeekBounds(%s) = [%s, %s), want [%s, %s)", tc.day, start, end, tc.start, tc.end)
			}
			if h := hoursBetween(t, start, end); h != tc.hours {
				t.Errorf("window length = %dh, want %dh", h, tc.hours)
			}
		})
	}
}

func TestClinicMonthBounds_AcrossDST(t *testing.T) {
	cases := []struct {
		day        string
		start, end string
		hours      int
	}{
		{"2026-03-15", "2026-02-28T22:00:00Z", "2026-03-31T21:00:00Z", 743},
		{"2026-04-15", "2026-03-31T21:00:00Z", "2026-04-30T21:00:00Z", 720},
		{"2026-10-24", "2026-09-30T21:00:00Z", "2026-10-31T22:00:00Z", 745},
		{"2027-03-28", "2027-02-28T22:00:00Z", "2027-03-31T21:00:00Z", 743},
		{"2027-10-30", "2027-09-30T21:00:00Z", "2027-10-31T22:00:00Z", 745},
	}
	for _, tc := range cases {
		t.Run(tc.day, func(t *testing.T) {
			start, end := ClinicMonthBounds(clinicTime(t, tc.day, 12, 0))
			if string(start) != tc.start || string(end) != tc.end {
				t.Fatalf("ClinicMonthBounds(%s) = [%s, %s), want [%s, %s)", tc.day, start, end, tc.start, tc.end)
			}
			if h := hoursBetween(t, start, end); h != tc.hours {
				t.Errorf("window length = %dh, want %dh", h, tc.hours)
			}
		})
	}
}

// Bare dates are UTC calendar days, not clinic days; RFC3339 values pass
// through untouched and an upper bound is already exclusive.
func TestRangeStartEnd_HalfOpenBounds(t *testing.T) {
	days := []struct{ day, next string }{
		{"2026-03-29", "2026-03-30"},
		{"2026-03-31", "2026-04-01"},
		{"2026-10-24", "2026-10-25"},
		{"2026-12-31", "2027-01-01"},
		{"2028-02-28", "2028-02-29"},
	}
	for _, d := range days {
		if got := RangeStart(d.day); got != d.day+"T00:00:00Z" {
			t.Errorf("RangeStart(%s) = %s", d.day, got)
		}
		if got, want := RangeEnd(d.day), RangeStart(d.next); got != want {
			t.Errorf("RangeEnd(%s) = %s, want the start of %s (%s)", d.day, got, d.next, want)
		}
	}

	if clinicStart, _ := ClinicDayBounds(clinicTime(t, "2026-10-24", 12, 0)); RangeStart("2026-10-24") == string(clinicStart) {
		t.Errorf("RangeStart of a bare date matched the clinic day start %s", clinicStart)
	}

	for _, s := range []string{
		"2026-03-28T22:00:00Z",
		"2026-03-29T21:00:00Z",
		"2026-03-29T00:00:00+03:00",
		"2026-02-30",
		"yesterday",
		"",
	} {
		if got := RangeStart(s); got != s {
			t.Errorf("RangeStart(%q) = %q, want it unchanged", s, got)
		}
		if got := RangeEnd(s); got != s {
			t.Errorf("RangeEnd(%q) = %q, want it unchanged", s, got)
		}
	}
}

// PreviousPeriod shifts by the window's duration, so across a DST switch the
// previous window is an hour off the clinic calendar, and a month's previous
// period is the same number of days rather than the previous month.
func TestPreviousPeriod_AcrossDST(t *testing.T) {
	dayOf := func(d string) [2]string {
		s, e := ClinicDayBounds(clinicTime(t, d, 12, 0))
		return [2]string{string(s), string(e)}
	}
	monthOf := func(d string) [2]string {
		s, e := ClinicMonthBounds(clinicTime(t, d, 12, 0))
		return [2]string{string(s), string(e)}
	}
	cases := []struct {
		name       string
		current    [2]string
		want       [2]string
		clinicPrev [2]string
		sameStart  bool
	}{
		{"after the 23-hour day", dayOf("2026-03-30"), [2]string{"2026-03-28T21:00:00Z", "2026-03-29T21:00:00Z"}, dayOf("2026-03-29"), false},
		{"the 23-hour day", dayOf("2026-03-29"), [2]string{"2026-03-27T22:00:00Z", "2026-03-28T22:00:00Z"}, dayOf("2026-03-28"), true},
		{"after the 25-hour day", dayOf("2026-10-25"), [2]string{"2026-10-23T22:00:00Z", "2026-10-24T22:00:00Z"}, dayOf("2026-10-24"), false},
		{"the 25-hour day", dayOf("2026-10-24"), [2]string{"2026-10-22T20:00:00Z", "2026-10-23T21:00:00Z"}, dayOf("2026-10-23"), false},
		{"April", monthOf("2026-04-15"), [2]string{"2026-03-01T21:00:00Z", "2026-03-31T21:00:00Z"}, monthOf("2026-03-15"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			from, to := PreviousPeriod(tc.current[0], tc.current[1])
			if from != tc.want[0] || to != tc.want[1] {
				t.Fatalf("PreviousPeriod(%s, %s) = [%s, %s), want [%s, %s)", tc.current[0], tc.current[1], from, to, tc.want[0], tc.want[1])
			}
			if to != tc.current[0] {
				t.Errorf("previous window ends at %s, want the current start %s", to, tc.current[0])
			}
			if (from == tc.clinicPrev[0]) != tc.sameStart {
				t.Errorf("previous start %s, clinic calendar start %s: same = %v, want %v", from, tc.clinicPrev[0], from == tc.clinicPrev[0], tc.sameStart)
			}
		})
	}

	if f, to := PreviousPeriod("2026-10-24", "2026-10-25"); f != "2026-10-22T00:00:00Z" || to != "2026-10-24T00:00:00Z" {
		t.Errorf("PreviousPeriod of bare dates = [%s, %s)", f, to)
	}
	for _, r := range [][2]string{
		{"2026-03-30T00:00:00Z", "2026-03-30T00:00:00Z"},
		{"2026-03-31T00:00:00Z", "2026-03-30T00:00:00Z"},
		{"2026-03-30T00:00:00Z", "not-a-date"},
	} {
		if f, to := PreviousPeriod(r[0], r[1]); f != "" || to != "" {
			t.Errorf("PreviousPeriod(%s, %s) = [%s, %s), want empty", r[0], r[1], f, to)
		}
	}
}

func mustInstant(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return v
}

// clinicTime is the given clinic-local wall clock time on day (YYYY-MM-DD).
func clinicTime(t *testing.T, day string, hour, minute int) time.Time {
	t.Helper()
	d, err := time.Parse(DateFormat, day)
	if err != nil {
		t.Fatalf("parse %q: %v", day, err)
	}
	return time.Date(d.Year(), d.Month(), d.Day(), hour, minute, 0, 0, ClinicLocation())
}

func hoursBetween(t *testing.T, start, end Date) int {
	t.Helper()
	return int(mustInstant(t, string(end)).Sub(mustInstant(t, string(start))) / time.Hour)
}
