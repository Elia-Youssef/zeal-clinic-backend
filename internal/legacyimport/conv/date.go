package conv

import (
	"fmt"
	"strings"
)

var months = map[string]int{
	"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6,
	"jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12,
}

const dobPivot = 26

func IsBlankDate(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		return true
	}
	for _, r := range v {
		if r != ' ' && r != '-' {
			return false
		}
	}
	return true
}

func parseDMY(v string, allowPast1900 bool) (y, m, d int, err error) {
	parts := strings.Split(strings.TrimSpace(v), "-")
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("not DD-Mon-YY: %q", v)
	}
	day, err := atoiStrict(parts[0])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("bad day in %q", v)
	}
	mon, ok := months[strings.ToLower(parts[1])]
	if !ok {
		return 0, 0, 0, fmt.Errorf("bad month in %q", v)
	}
	yy, err := atoiStrict(parts[2])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("bad year in %q", v)
	}
	switch {
	case len(parts[2]) == 4:

	case allowPast1900:
		if yy <= dobPivot {
			yy += 2000
		} else {
			yy += 1900
		}
	default:
		yy += 2000
	}
	if day < 1 || day > 31 {
		return 0, 0, 0, fmt.Errorf("day out of range in %q", v)
	}
	return yy, mon, day, nil
}

func Date(v string, isDOB bool) (out string, ok bool, err error) {
	if IsBlankDate(v) {
		return "", false, nil
	}
	y, m, d, err := parseDMY(v, isDOB)
	if err != nil {
		return "", false, err
	}
	return fmt.Sprintf("%04d-%02d-%02d", y, m, d), true, nil
}

func DateTimeZ(v string) (out string, ok bool, err error) {
	d, ok, err := Date(v, false)
	if !ok || err != nil {
		return "", ok, err
	}
	return d + "T00:00:00Z", true, nil
}

func AppointmentDateTime(date, hhmm string) (out string, ok bool, err error) {
	d, dok, err := Date(date, false)
	if !dok || err != nil {
		return "", dok, err
	}
	hh, mm, terr := parseHHMM(hhmm)
	if terr != nil {
		return "", false, terr
	}
	return fmt.Sprintf("%sT%02d:%02d:00Z", d, hh, mm), true, nil
}

func parseHHMM(v string) (h, m int, err error) {
	v = strings.TrimSpace(v)
	parts := strings.Split(v, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("not HH:MM: %q", v)
	}
	h, err = atoiStrict(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("bad hour in %q", v)
	}
	m, err = atoiStrict(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("bad minute in %q", v)
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("time out of range: %q", v)
	}
	return h, m, nil
}

func atoiStrict(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("non-digit in %q", s)
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}
