package handlers

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	phoneRegex = regexp.MustCompile(`^[\+]?[0-9\s\-\(\)]{7,20}$`)
	dateRegex  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

// ValidationError collects multiple field errors
type ValidationError struct {
	Fields map[string]string
}

func (v *ValidationError) Error() string {
	parts := make([]string, 0, len(v.Fields))
	for field, msg := range v.Fields {
		parts = append(parts, fmt.Sprintf("%s: %s", field, msg))
	}
	return strings.Join(parts, "; ")
}

func (v *ValidationError) HasErrors() bool {
	return len(v.Fields) > 0
}

func NewValidator() *ValidationError {
	return &ValidationError{Fields: make(map[string]string)}
}

func (v *ValidationError) Required(field, value, label string) {
	if strings.TrimSpace(value) == "" {
		v.Fields[field] = label + " is required"
	}
}

func (v *ValidationError) Email(field, value string) {
	if value == "" {
		return // use Required() for mandatory check
	}
	if !emailRegex.MatchString(value) {
		v.Fields[field] = "invalid email format"
	}
}

func (v *ValidationError) Phone(field, value string) {
	if value == "" {
		return
	}
	if !phoneRegex.MatchString(value) {
		v.Fields[field] = "invalid phone format (7-20 digits, may include +, spaces, dashes)"
	}
}

func (v *ValidationError) Date(field, value string) {
	if value == "" {
		return
	}
	if !dateRegex.MatchString(value) {
		v.Fields[field] = "invalid date format (expected YYYY-MM-DD)"
		return
	}
	if _, err := time.Parse("2006-01-02", value); err != nil {
		v.Fields[field] = "invalid date"
	}
}

func (v *ValidationError) DateTime(field, value string) {
	if value == "" {
		return
	}
	// Accept RFC3339 or ISO 8601 formats
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
		if _, err := time.Parse(layout, value); err == nil {
			return
		}
	}
	v.Fields[field] = "invalid datetime format (expected ISO 8601)"
}

func (v *ValidationError) OneOf(field, value string, allowed []string, label string) {
	if value == "" {
		return
	}
	for _, a := range allowed {
		if value == a {
			return
		}
	}
	v.Fields[field] = fmt.Sprintf("%s must be one of: %s", label, strings.Join(allowed, ", "))
}

func (v *ValidationError) MinLength(field, value string, min int, label string) {
	if len(strings.TrimSpace(value)) < min {
		v.Fields[field] = fmt.Sprintf("%s must be at least %d characters", label, min)
	}
}

func (v *ValidationError) Positive(field string, value float64, label string) {
	if value < 0 {
		v.Fields[field] = fmt.Sprintf("%s cannot be negative", label)
	}
}
