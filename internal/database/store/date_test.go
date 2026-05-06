package store

import (
	"strings"
	"testing"
	"time"
)

func TestDate_NowAndToday(t *testing.T) {
	t.Run("DateNow uses RFC3339 format", func(t *testing.T) {
		d := DateNow()
		if _, err := time.Parse(time.RFC3339, string(d)); err != nil {
			t.Errorf("DateNow %q not parseable as RFC3339: %v", d, err)
		}
	})
	t.Run("DateToday is YYYY-MM-DD only", func(t *testing.T) {
		d := DateToday()
		if _, err := time.Parse("2006-01-02", string(d)); err != nil {
			t.Errorf("DateToday %q not parseable: %v", d, err)
		}
		if strings.Contains(string(d), "T") {
			t.Errorf("DateToday should not include time component, got %q", d)
		}
	})
	t.Run("DateFrom round-trips a known time", func(t *testing.T) {
		ref := time.Date(2024, 5, 17, 10, 30, 0, 0, time.UTC)
		d := DateFrom(ref)
		got, err := time.Parse(time.RFC3339, string(d))
		if err != nil {
			t.Fatalf("DateFrom not parseable: %v", err)
		}
		if !got.Equal(ref) {
			t.Errorf("DateFrom round-trip: got %v want %v", got, ref)
		}
	})
}

func TestDate_StringAndIsZero(t *testing.T) {
	if Date("hello").String() != "hello" {
		t.Errorf("String() should return raw underlying value")
	}
	if !Date("").IsZero() {
		t.Errorf("empty Date should be zero")
	}
	if Date("x").IsZero() {
		t.Errorf("non-empty Date should not be zero")
	}
}

func TestDate_Time(t *testing.T) {
	t.Run("zero returns zero time and no error", func(t *testing.T) {
		got, err := Date("").Time()
		if err != nil {
			t.Errorf("unexpected err: %v", err)
		}
		if !got.IsZero() {
			t.Errorf("expected zero time, got %v", got)
		}
	})
	t.Run("RFC3339 parses", func(t *testing.T) {
		_, err := Date("2024-01-02T03:04:05Z").Time()
		if err != nil {
			t.Errorf("RFC3339 parse failed: %v", err)
		}
	})
	t.Run("date-only parses", func(t *testing.T) {
		got, err := Date("2024-01-02").Time()
		if err != nil {
			t.Errorf("date-only parse failed: %v", err)
		}
		if got.Year() != 2024 || got.Month() != 1 || got.Day() != 2 {
			t.Errorf("got %v", got)
		}
	})
	t.Run("garbage returns error", func(t *testing.T) {
		_, err := Date("not-a-date").Time()
		if err == nil {
			t.Errorf("expected err")
		}
	})
}

func TestDate_BeforeAfter(t *testing.T) {
	a := Date("2024-01-01T00:00:00Z")
	b := Date("2024-01-02T00:00:00Z")
	if !a.Before(b) {
		t.Errorf("a should be before b")
	}
	if a.After(b) {
		t.Errorf("a should not be after b")
	}
	if a.Before(a) || a.After(a) {
		t.Errorf("a is neither before nor after itself")
	}
	t.Run("mixed formats compare correctly", func(t *testing.T) {
		x := Date("2024-01-01")
		y := Date("2024-01-02T00:00:00Z")
		if !x.Before(y) {
			t.Errorf("date-only x should be before RFC3339 y")
		}
		if !y.After(x) {
			t.Errorf("y should be after x")
		}
	})
	t.Run("falls back to lexical compare on parse failure", func(t *testing.T) {
		// Both unparseable: lex order applies.
		if !Date("aaa").Before(Date("bbb")) {
			t.Errorf("lex fallback failed")
		}
		if !Date("bbb").After(Date("aaa")) {
			t.Errorf("lex fallback failed")
		}
	})
}

func TestDate_DateOnly(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"2024-05-17T10:30:00Z", "2024-05-17"},
		{"2024-05-17", "2024-05-17"},
		// Unparseable: returns the raw input.
		{"garbage", "garbage"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := Date(tc.in).DateOnly(); got != tc.want {
				t.Errorf("DateOnly(%q)=%q want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestDate_Scan(t *testing.T) {
	t.Run("nil clears", func(t *testing.T) {
		d := Date("prev")
		if err := (&d).Scan(nil); err != nil {
			t.Fatal(err)
		}
		if d != "" {
			t.Errorf("expected empty after Scan(nil), got %q", d)
		}
	})
	t.Run("string", func(t *testing.T) {
		var d Date
		if err := (&d).Scan("2024-01-01"); err != nil {
			t.Fatal(err)
		}
		if d != "2024-01-01" {
			t.Errorf("got %q", d)
		}
	})
	t.Run("byte slice", func(t *testing.T) {
		var d Date
		if err := (&d).Scan([]byte("2024-02-02")); err != nil {
			t.Fatal(err)
		}
		if d != "2024-02-02" {
			t.Errorf("got %q", d)
		}
	})
	t.Run("time.Time becomes RFC3339", func(t *testing.T) {
		var d Date
		ref := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
		if err := (&d).Scan(ref); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(d), "2024-06-01T12:00:00") {
			t.Errorf("Scan(time.Time) -> %q", d)
		}
	})
	t.Run("unsupported type returns error", func(t *testing.T) {
		var d Date
		if err := (&d).Scan(42); err == nil {
			t.Errorf("expected error for int")
		}
	})
}

func TestDate_Value(t *testing.T) {
	v, err := Date("2024-01-01").Value()
	if err != nil {
		t.Fatal(err)
	}
	s, ok := v.(string)
	if !ok {
		t.Fatalf("Value should return string, got %T", v)
	}
	if s != "2024-01-01" {
		t.Errorf("got %q", s)
	}
	t.Run("empty Value returns empty string", func(t *testing.T) {
		v, err := Date("").Value()
		if err != nil {
			t.Fatal(err)
		}
		if v != "" {
			t.Errorf("got %v", v)
		}
	})
}
