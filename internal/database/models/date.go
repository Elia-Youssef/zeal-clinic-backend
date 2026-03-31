package models

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

func (d Date) Before(other Date) bool {
	t1, e1 := d.Time()
	t2, e2 := other.Time()
	if e1 != nil || e2 != nil {
		return string(d) < string(other)
	}
	return t1.Before(t2)
}

func (d Date) After(other Date) bool {
	t1, e1 := d.Time()
	t2, e2 := other.Time()
	if e1 != nil || e2 != nil {
		return string(d) > string(other)
	}
	return t1.After(t2)
}

func (d Date) DateOnly() string {
	t, err := d.Time()
	if err != nil {
		return string(d)
	}
	return t.Format(DateFormat)
}

func (d *Date) Scan(value interface{}) error {
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
