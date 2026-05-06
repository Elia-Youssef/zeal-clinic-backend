package validation

import (
	"strings"
	"testing"
)

func TestErrors_Error(t *testing.T) {
	t.Run("empty map returns empty string", func(t *testing.T) {
		var e Errors = Errors{}
		if got := e.Error(); got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})

	t.Run("single field formats as field: msg", func(t *testing.T) {
		e := Errors{"name": "is required"}
		if got := e.Error(); got != "name: is required" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("multiple fields joined by '; '", func(t *testing.T) {
		e := Errors{"a": "x", "b": "y"}
		got := e.Error()
		// Map ordering is not deterministic, so check contents.
		if !strings.Contains(got, "a: x") || !strings.Contains(got, "b: y") {
			t.Errorf("missing field, got %q", got)
		}
		if !strings.Contains(got, "; ") {
			t.Errorf("expected '; ' separator, got %q", got)
		}
	})

	t.Run("Errors satisfies the error interface", func(t *testing.T) {
		var _ error = Errors{}
	})
}

func TestRequired(t *testing.T) {
	cases := []struct {
		name, value, label, want string
	}{
		{"empty string", "", "Name", "Name is required"},
		{"whitespace only space", " ", "Name", "Name is required"},
		{"whitespace tabs/newlines", "\t\n ", "Name", "Name is required"},
		{"non-empty value", "x", "Name", ""},
		{"value with surrounding ws is ok", "  x  ", "Name", ""},
		{"label preserved verbatim", "", "Display name", "Display name is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Required(tc.value, tc.label); got != tc.want {
				t.Errorf("Required(%q,%q)=%q want %q", tc.value, tc.label, got, tc.want)
			}
		})
	}
}

func TestEmail(t *testing.T) {
	valid := []string{
		"a@b.co",
		"first.last@example.com",
		"user+tag@domain.io",
		"a_b-c.d%e@x-y.z.org",
		"User@Example.COM",
		"123@456.com",
	}
	invalid := []string{
		"plain",
		"@example.com",
		"a@",
		"a@b",        // tld too short
		"a@b.c",      // tld 1 char
		"a b@c.com",  // space in local
		"a@b .com",   // space in domain
		"a@@b.com",   // double @
		"a@b.com ",   // trailing space
		" a@b.com",   // leading space
		"a@b.123",    // digits-only tld
	}
	t.Run("empty is valid (treated as optional)", func(t *testing.T) {
		if got := Email(""); got != "" {
			t.Errorf("empty should be ok, got %q", got)
		}
	})
	for _, v := range valid {
		t.Run("valid:"+v, func(t *testing.T) {
			if got := Email(v); got != "" {
				t.Errorf("Email(%q) wanted ok, got %q", v, got)
			}
		})
	}
	for _, v := range invalid {
		t.Run("invalid:"+v, func(t *testing.T) {
			if got := Email(v); got == "" {
				t.Errorf("Email(%q) wanted error, got ok", v)
			} else if got != "invalid email format" {
				t.Errorf("Email(%q)=%q want %q", v, got, "invalid email format")
			}
		})
	}
}

func TestPhone(t *testing.T) {
	valid := []string{
		"1234567",
		"+961 70 123 456",
		"(03) 123-456",
		"+1-800-555-1212",
		"12345678901234567890", // 20 chars, max length
		"+12345 67",
	}
	invalid := []string{
		"123",                    // too short (<7)
		"123456",                 // 6 chars, just below
		"123456789012345678901",  // 21 chars, just above max
		"abcdefg",                // letters
		"070-abc-1234",           // mixed letters
		"+961.70.123.456",        // dot separator not allowed
		"070_123_456",            // underscore not allowed
	}
	t.Run("empty is valid", func(t *testing.T) {
		if got := Phone(""); got != "" {
			t.Errorf("Phone empty should be ok, got %q", got)
		}
	})
	for _, v := range valid {
		t.Run("valid:"+v, func(t *testing.T) {
			if got := Phone(v); got != "" {
				t.Errorf("Phone(%q) wanted ok, got %q", v, got)
			}
		})
	}
	for _, v := range invalid {
		t.Run("invalid:"+v, func(t *testing.T) {
			if got := Phone(v); got == "" {
				t.Errorf("Phone(%q) wanted error", v)
			}
		})
	}
}

func TestDate(t *testing.T) {
	t.Run("empty is valid", func(t *testing.T) {
		if got := Date(""); got != "" {
			t.Errorf("expected empty to be ok, got %q", got)
		}
	})
	valid := []string{
		"2024-01-01",
		"1999-12-31",
		"2000-02-29", // leap day
	}
	for _, v := range valid {
		t.Run("valid:"+v, func(t *testing.T) {
			if got := Date(v); got != "" {
				t.Errorf("Date(%q)=%q want ok", v, got)
			}
		})
	}

	t.Run("wrong format short year", func(t *testing.T) {
		if got := Date("99-12-31"); got != "invalid date format (expected YYYY-MM-DD)" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("wrong format slash separator", func(t *testing.T) {
		if got := Date("2024/01/01"); got == "" {
			t.Errorf("expected error")
		}
	})
	t.Run("wrong format with time component", func(t *testing.T) {
		if got := Date("2024-01-01T00:00:00Z"); got == "" {
			t.Errorf("expected error")
		}
	})
	t.Run("right shape but invalid date", func(t *testing.T) {
		// Passes regex (\d{4}-\d{2}-\d{2}) but fails time.Parse strict mode
		if got := Date("2024-13-01"); got != "invalid date" {
			t.Errorf("Date(2024-13-01)=%q want %q", got, "invalid date")
		}
	})
	t.Run("right shape but day overflow", func(t *testing.T) {
		if got := Date("2024-02-30"); got != "invalid date" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("non-leap-year feb 29", func(t *testing.T) {
		if got := Date("2023-02-29"); got != "invalid date" {
			t.Errorf("got %q", got)
		}
	})
}

func TestDateTime(t *testing.T) {
	t.Run("empty is valid", func(t *testing.T) {
		if got := DateTime(""); got != "" {
			t.Errorf("got %q", got)
		}
	})
	valid := []string{
		"2024-01-01T00:00:00Z",
		"2024-01-01T15:04:05+02:00",
		"2024-01-01T15:04:05",     // local layout 1
		"2024-01-01 15:04:05",     // local layout 2
	}
	for _, v := range valid {
		t.Run("valid:"+v, func(t *testing.T) {
			if got := DateTime(v); got != "" {
				t.Errorf("DateTime(%q)=%q want ok", v, got)
			}
		})
	}

	invalid := []string{
		"2024-01-01",            // date only
		"15:04:05",              // time only
		"not-a-date",
		"2024/01/01 15:04:05",   // wrong separators
		"2024-01-01T25:00:00Z",  // hour overflow
	}
	for _, v := range invalid {
		t.Run("invalid:"+v, func(t *testing.T) {
			if got := DateTime(v); got != "invalid datetime format (expected ISO 8601)" {
				t.Errorf("DateTime(%q)=%q want invalid datetime", v, got)
			}
		})
	}
}

func TestOneOf(t *testing.T) {
	allowed := []string{"a", "b", "c"}
	t.Run("empty is valid", func(t *testing.T) {
		if got := OneOf("", allowed, "Letter"); got != "" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("matches first", func(t *testing.T) {
		if got := OneOf("a", allowed, "Letter"); got != "" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("matches last", func(t *testing.T) {
		if got := OneOf("c", allowed, "Letter"); got != "" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("case sensitive mismatch", func(t *testing.T) {
		if got := OneOf("A", allowed, "Letter"); got == "" {
			t.Errorf("expected error for case mismatch")
		}
	})
	t.Run("not in allowed", func(t *testing.T) {
		want := "Letter must be one of: a, b, c"
		if got := OneOf("d", allowed, "Letter"); got != want {
			t.Errorf("got %q want %q", got, want)
		}
	})
	t.Run("empty allowed list rejects any non-empty value", func(t *testing.T) {
		if got := OneOf("x", []string{}, "X"); got == "" {
			t.Errorf("expected error")
		}
	})
}

func TestMinLength(t *testing.T) {
	cases := []struct {
		name, value string
		min         int
		label       string
		wantErr     bool
	}{
		{"meets min", "abc", 3, "Name", false},
		{"exceeds min", "abcd", 3, "Name", false},
		{"below min", "ab", 3, "Name", true},
		{"empty below min", "", 1, "Name", true},
		{"min zero accepts empty", "", 0, "Name", false},
		{"trims whitespace before counting", "   ", 1, "Name", true},
		{"trims surrounding whitespace", "  ab  ", 3, "Name", true}, // trimmed -> 2 chars
		{"trimmed value meets min", "  abc  ", 3, "Name", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MinLength(tc.value, tc.min, tc.label)
			if tc.wantErr && got == "" {
				t.Errorf("expected error")
			}
			if !tc.wantErr && got != "" {
				t.Errorf("expected ok, got %q", got)
			}
		})
	}
}

func TestPositive(t *testing.T) {
	cases := []struct {
		name    string
		value   float64
		wantErr bool
	}{
		{"zero is allowed", 0, false},
		{"positive int", 5, false},
		{"positive float", 0.001, false},
		{"large positive", 1e9, false},
		{"negative", -1, true},
		{"tiny negative", -0.0001, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Positive(tc.value, "Amount")
			if tc.wantErr && got == "" {
				t.Errorf("expected error")
			}
			if !tc.wantErr && got != "" {
				t.Errorf("expected ok, got %q", got)
			}
		})
	}
	t.Run("error message uses label", func(t *testing.T) {
		if got := Positive(-1, "Foo"); got != "Foo cannot be negative" {
			t.Errorf("got %q", got)
		}
	})
}
