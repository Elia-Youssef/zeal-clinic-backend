package conv

import (
	"testing"
	"time"

	"clinic-api/internal/database/store"
)

// The DST switches used below, checked against the embedded tz database
// first: each instant is the first second on the new offset.
func TestClinicTimezone_DSTVectors(t *testing.T) {
	for _, v := range []struct {
		at            string
		before, after int
	}{
		{"2025-03-29T22:00:00Z", 2, 3},
		{"2025-10-25T21:00:00Z", 3, 2},
		{"2026-03-28T22:00:00Z", 2, 3},
		{"2026-10-24T21:00:00Z", 3, 2},
	} {
		at, err := time.Parse(time.RFC3339, v.at)
		if err != nil {
			t.Fatal(err)
		}
		_, before := at.Add(-time.Second).In(store.ClinicLocation()).Zone()
		_, after := at.In(store.ClinicLocation()).Zone()
		if before != v.before*3600 || after != v.after*3600 {
			t.Errorf("offsets around %s = %d/%d s, want %d/%d h", v.at, before, after, v.before, v.after)
		}
	}
}

// Legacy appointment times are Beirut wall clock and are stored as UTC.
func TestAppointmentDateTime_BeirutToUTC(t *testing.T) {
	cases := []struct{ date, hhmm, want string }{
		{"28-Mar-26", "10:00", "2026-03-28T08:00:00Z"},
		{"30-Mar-26", "10:00", "2026-03-30T07:00:00Z"},
		// 00:30 on 29 March 2026 does not exist and lands on the same instant
		// as 01:30.
		{"29-Mar-26", "00:30", "2026-03-28T22:30:00Z"},
		{"29-Mar-26", "01:30", "2026-03-28T22:30:00Z"},
		{"24-Oct-26", "22:30", "2026-10-24T19:30:00Z"},
		// 23:30 on 24 October 2026 happens twice; the second pass is used.
		{"24-Oct-26", "23:30", "2026-10-24T21:30:00Z"},
		{"25-Oct-26", "00:30", "2026-10-24T22:30:00Z"},
		{"25-Oct-25", "23:30", "2025-10-25T21:30:00Z"},
		{"10-Jan-2099", "09:00", "2099-01-10T07:00:00Z"},
		{"15-Jul-2099", "09:00", "2099-07-15T06:00:00Z"},
		{" 5-jan-26 ", " 7:05 ", "2026-01-05T05:05:00Z"},
		{"31-Feb-26", "10:00", "2026-03-03T08:00:00Z"},
	}
	for _, c := range cases {
		got, ok, err := AppointmentDateTime(c.date, c.hhmm)
		if err != nil || !ok || got != c.want {
			t.Errorf("AppointmentDateTime(%q, %q) = %q, %v, %v; want %q", c.date, c.hhmm, got, ok, err, c.want)
		}
	}

	for _, blank := range []string{"", "  ", "--", "- -"} {
		if got, ok, err := AppointmentDateTime(blank, "10:00"); got != "" || ok || err != nil {
			t.Errorf("blank date %q = %q, %v, %v; want empty, not ok, no error", blank, got, ok, err)
		}
	}
	for _, bad := range [][2]string{
		{"28-Mar-26", "24:00"},
		{"28-Mar-26", "10"},
		{"28-Mar-26", "1a:00"},
		{"28-Mar-26", ""},
		{"32-Mar-26", "10:00"},
		{"28-Xyz-26", "10:00"},
		{"28-Mar", "10:00"},
		{"2026-03-28", "10:00"},
	} {
		if got, ok, err := AppointmentDateTime(bad[0], bad[1]); err == nil || ok || got != "" {
			t.Errorf("AppointmentDateTime(%q, %q) = %q, %v, %v; want an error", bad[0], bad[1], got, ok, err)
		}
	}
}

// Two-digit years are 20xx, except birth dates, which switch to 19xx above 26.
// Days are only checked to be 1-31.
func TestDate_TwoDigitYearsAndDays(t *testing.T) {
	cases := []struct {
		in    string
		isDOB bool
		want  string
	}{
		{"15-Jun-26", true, "2026-06-15"},
		{"15-Jun-27", true, "1927-06-15"},
		{"15-Jun-27", false, "2027-06-15"},
		{"05-Jan-90", true, "1990-01-05"},
		{"05-Jan-90", false, "2090-01-05"},
		{"15-Jun-1927", false, "1927-06-15"},
		{"01-DEC-00", true, "2000-12-01"},
		{"31-Feb-26", false, "2026-02-31"},
	}
	for _, c := range cases {
		got, ok, err := Date(c.in, c.isDOB)
		if err != nil || !ok || got != c.want {
			t.Errorf("Date(%q, %v) = %q, %v, %v; want %q", c.in, c.isDOB, got, ok, err, c.want)
		}
	}
	if got, ok, err := DateTimeZ("15-Mar-26"); err != nil || !ok || got != "2026-03-15T00:00:00Z" {
		t.Errorf("DateTimeZ = %q, %v, %v", got, ok, err)
	}
	for _, bad := range []string{"00-Jan-26", "32-Jan-26", "15-Foo-26", "15-Jun-2x", "15/06/26"} {
		if _, _, err := Date(bad, true); err == nil {
			t.Errorf("Date(%q) accepted", bad)
		}
	}
}

func TestNormalizers(t *testing.T) {
	for in, want := range map[string]string{"m": "Male", " MALE ": "Male", "F": "Female", "female": "Female", "": "", "x": ""} {
		if got := NormalizeGender(in); got != want {
			t.Errorf("NormalizeGender(%q) = %q, want %q", in, got, want)
		}
	}
	for _, c := range []struct {
		in, want     string
		unrecognized bool
	}{
		{"0+", "O+", false}, {"o-", "O-", false}, {"ab+", "AB+", false}, {"b+", "B+", false},
		{"A-", "A-", false}, {"", "", false}, {"-", "", false}, {"XY", "", true}, {"A", "", true},
	} {
		got, unrec := NormalizeBloodType(c.in)
		if got != c.want || unrec != c.unrecognized {
			t.Errorf("NormalizeBloodType(%q) = %q, %v; want %q, %v", c.in, got, unrec, c.want, c.unrecognized)
		}
	}
	for in, want := range map[string]Name{
		"":                    {},
		"Alpha":               {First: "Alpha"},
		"Alpha Tester":        {First: "Alpha", Last: "Tester"},
		"Alpha Q R Tester":    {First: "Alpha", Middle: "Q R", Last: "Tester"},
		" Alpha  .  Tester -": {First: "Alpha", Last: "Tester"},
	} {
		if got := ParseFullName(in); got != want {
			t.Errorf("ParseFullName(%q) = %+v, want %+v", in, got, want)
		}
	}
	for in, want := range map[string]bool{"Al": true, ".-": false, "12": false, "": false, "\u00e9": true} {
		if got := HasLetter(in); got != want {
			t.Errorf("HasLetter(%q) = %v", in, got)
		}
	}
}

func TestNumbers(t *testing.T) {
	for _, c := range []struct {
		in   string
		want float64
		ok   bool
	}{{"", 7, true}, {" 12.5 ", 12.5, true}, {"-3", -3, true}, {"abc", 7, false}, {"1,000", 7, false}} {
		if got, ok := Float(c.in, 7); got != c.want || ok != c.ok {
			t.Errorf("Float(%q) = %v, %v; want %v, %v", c.in, got, ok, c.want, c.ok)
		}
	}
	for _, c := range []struct {
		in   string
		want int
		ok   bool
	}{{"", 1, true}, {"12", 12, true}, {"12.9", 12, true}, {"-2", -2, true}, {"x", 1, false}} {
		if got, ok := Int(c.in, 1); got != c.want || ok != c.ok {
			t.Errorf("Int(%q) = %v, %v; want %v, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}
