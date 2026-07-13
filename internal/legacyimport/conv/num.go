package conv

import (
	"strconv"
	"strings"
)

func Float(v string, def float64) (float64, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return def, true
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def, false
	}
	return f, true
}

func Int(v string, def int) (int, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return def, true
	}
	if n, err := strconv.Atoi(v); err == nil {
		return n, true
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil {
		return int(f), true
	}
	return def, false
}
