package validation

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Err is the validation sentinel. Both kinds of validation error a validator
// returns match it through errors.Is: the sentinel wrapped with a
// user-facing message, and a field-level Errors map.
var Err = errors.New("validation")

// Errors holds field-level validation errors. Implements error so IsValid
// methods can return a plain error while handlers can still serialize the map.
type Errors map[string]string

func (e Errors) Error() string {
	parts := make([]string, 0, len(e))
	for field, msg := range e {
		parts = append(parts, fmt.Sprintf("%s: %s", field, msg))
	}
	return strings.Join(parts, "; ")
}

// Is reports that a field-level error map is a validation error, so both
// validation kinds answer one errors.Is check against Err.
func (e Errors) Is(target error) bool {
	return target == Err
}

var (
	emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	phoneRegex = regexp.MustCompile(`^[\+]?[0-9\s\-\(\)]{7,20}$`)
	dateRegex  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

func Required(value, label string) string {
	if strings.TrimSpace(value) == "" {
		return label + " is required"
	}
	return ""
}

func Email(value string) string {
	if value == "" {
		return ""
	}
	if !emailRegex.MatchString(value) {
		return "invalid email format"
	}
	return ""
}

func Phone(value string) string {
	if value == "" {
		return ""
	}
	if !phoneRegex.MatchString(value) {
		return "invalid phone format (7-20 digits, may include +, spaces, dashes)"
	}
	return ""
}

func Date(value string) string {
	if value == "" {
		return ""
	}
	if !dateRegex.MatchString(value) {
		return "invalid date format (expected YYYY-MM-DD)"
	}
	if _, err := time.Parse("2006-01-02", value); err != nil {
		return "invalid date"
	}
	return ""
}

func DateTime(value string) string {
	if value == "" {
		return ""
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
		if _, err := time.Parse(layout, value); err == nil {
			return ""
		}
	}
	return "invalid datetime format (expected ISO 8601)"
}

func OneOf(value string, allowed []string, label string) string {
	if value == "" {
		return ""
	}
	for _, a := range allowed {
		if value == a {
			return ""
		}
	}
	return fmt.Sprintf("%s must be one of: %s", label, strings.Join(allowed, ", "))
}

func MinLength(value string, min int, label string) string {
	if len(strings.TrimSpace(value)) < min {
		return fmt.Sprintf("%s must be at least %d characters", label, min)
	}
	return ""
}

// NormalizePhone strips all whitespace from a phone value.
func NormalizePhone(s string) string {
	return strings.Join(strings.Fields(s), "")
}

func Positive(value float64, label string) string {
	if value < 0 {
		return fmt.Sprintf("%s cannot be negative", label)
	}
	return ""
}

func GreaterThanZero(value float64, label string) string {
	if value <= 0 {
		return fmt.Sprintf("%s must be greater than 0", label)
	}
	return ""
}
