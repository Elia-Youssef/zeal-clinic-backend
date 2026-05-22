package validation

import (
	"strings"
	"testing"
)

// Errors formats field/message pairs joined by "; " and satisfies error.
func TestErrors_Error(t *testing.T) {
	if got := (Errors{}).Error(); got != "" {
		t.Errorf("empty Errors = %q want empty", got)
	}
	if got := (Errors{"name": "is required"}).Error(); got != "name: is required" {
		t.Errorf("single = %q", got)
	}
	got := Errors{"a": "x", "b": "y"}.Error() // map order non-deterministic
	if !strings.Contains(got, "a: x") || !strings.Contains(got, "b: y") || !strings.Contains(got, "; ") {
		t.Errorf("multi = %q", got)
	}
}

func TestRequired(t *testing.T) {
	// Blank (after trimming) is an error that names the label; anything else is ok.
	if got := Required("  ", "Display name"); got != "Display name is required" {
		t.Errorf("blank = %q", got)
	}
	if got := Required("  x  ", "Name"); got != "" {
		t.Errorf("non-blank should pass, got %q", got)
	}
}

func TestEmail(t *testing.T) {
	ok := []string{"", "a@b.co", "first.last@example.com", "User@Example.COM"}
	bad := []string{"plain", "@example.com", "a@b", "a b@c.com", "a@@b.com", " a@b.com"}
	for _, v := range ok {
		if got := Email(v); got != "" {
			t.Errorf("Email(%q) should pass, got %q", v, got)
		}
	}
	for _, v := range bad {
		if Email(v) == "" {
			t.Errorf("Email(%q) should fail", v)
		}
	}
}

func TestPhone(t *testing.T) {
	ok := []string{"", "1234567", "+961 70 123 456", "+1-800-555-1212"}
	bad := []string{"123", "abcdefg", "+961.70.123.456", "123456789012345678901"} // too short / letters / dots / too long
	for _, v := range ok {
		if got := Phone(v); got != "" {
			t.Errorf("Phone(%q) should pass, got %q", v, got)
		}
	}
	for _, v := range bad {
		if Phone(v) == "" {
			t.Errorf("Phone(%q) should fail", v)
		}
	}
}

func TestDate(t *testing.T) {
	if got := Date(""); got != "" {
		t.Errorf("empty should pass, got %q", got)
	}
	for _, v := range []string{"2024-01-01", "2000-02-29"} { // incl. leap day
		if got := Date(v); got != "" {
			t.Errorf("Date(%q) should pass, got %q", v, got)
		}
	}
	// Wrong shape vs. right shape but impossible date use distinct messages.
	if got := Date("2024/01/01"); got != "invalid date format (expected YYYY-MM-DD)" {
		t.Errorf("bad format msg = %q", got)
	}
	if got := Date("2024-13-01"); got != "invalid date" {
		t.Errorf("impossible date msg = %q", got)
	}
}

func TestDateTime(t *testing.T) {
	if got := DateTime(""); got != "" {
		t.Errorf("empty should pass, got %q", got)
	}
	for _, v := range []string{"2024-01-01T00:00:00Z", "2024-01-01T15:04:05+02:00", "2024-01-01 15:04:05"} {
		if got := DateTime(v); got != "" {
			t.Errorf("DateTime(%q) should pass, got %q", v, got)
		}
	}
	if got := DateTime("2024-01-01"); got != "invalid datetime format (expected ISO 8601)" {
		t.Errorf("date-only should fail datetime, got %q", got)
	}
}

func TestOneOf(t *testing.T) {
	allowed := []string{"a", "b", "c"}
	if OneOf("", allowed, "X") != "" || OneOf("a", allowed, "X") != "" {
		t.Error("empty and allowed values should pass")
	}
	if OneOf("A", allowed, "X") == "" {
		t.Error("OneOf is case-sensitive; 'A' should fail")
	}
	if got := OneOf("d", allowed, "Letter"); got != "Letter must be one of: a, b, c" {
		t.Errorf("message = %q", got)
	}
}

func TestMinLength(t *testing.T) {
	// Length is measured after trimming whitespace.
	if MinLength("abc", 3, "Name") != "" {
		t.Error("exactly-min should pass")
	}
	if MinLength("  ab  ", 3, "Name") == "" {
		t.Error("trimmed length 2 < 3 should fail")
	}
	if MinLength("", 0, "Name") != "" {
		t.Error("min 0 should accept empty")
	}
}

func TestPositive(t *testing.T) {
	if Positive(0, "Amount") != "" || Positive(5, "Amount") != "" {
		t.Error("zero and positive should pass")
	}
	if got := Positive(-1, "Foo"); got != "Foo cannot be negative" {
		t.Errorf("negative message = %q", got)
	}
}
