package pdf

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"clinic-api/internal/config"
	"clinic-api/internal/database/store"

	"github.com/johnfercher/maroto/v2/pkg/core"
)

// clinicDate formats a stored UTC timestamp in the clinic's local timezone
// (Beirut) as DD/MM/YYYY, the form staff read on invoices and reports.
func clinicDate(d store.Date) string {
	if t, err := d.Time(); err == nil {
		return t.In(store.ClinicLocation()).Format("02/01/2006")
	}
	return d.DateOnly()
}

// TmpDir returns the PDF cache directory: a tmp/ subfolder next to the
// database file.
func TmpDir() string {
	dir := filepath.Join(config.DataDir(), "tmp")
	_ = os.MkdirAll(dir, 0700)
	return dir
}

// tmpPath is <name>-<unix milliseconds>-<random suffix>.pdf in the cache
// folder; the suffix keeps two files made in the same millisecond apart.
func tmpPath(name string) string {
	suffix := make([]byte, 3)
	_, _ = rand.Read(suffix)
	return filepath.Join(TmpDir(), fmt.Sprintf("%s-%d-%s.pdf", name, time.Now().UnixMilli(), hex.EncodeToString(suffix)))
}

// rangeFileDates names the clinic-local calendar days a report range covers,
// for the file name. A bare YYYY-MM-DD bound is a calendar day already; an
// RFC3339 instant takes the clinic-local date it falls on, and the exclusive
// upper instant steps back a second onto the last day it includes.
func rangeFileDates(from, to string) (string, string, error) {
	start, err := boundDay(from, 0)
	if err != nil {
		return "", "", err
	}
	end, err := boundDay(to, -time.Second)
	if err != nil {
		return "", "", err
	}
	return start, end, nil
}

func boundDay(v string, shift time.Duration) (string, error) {
	if len(v) == len(store.DateFormat) {
		if _, err := time.Parse(store.DateFormat, v); err == nil {
			return v, nil
		}
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return "", fmt.Errorf("range bound %q is neither a date nor an instant", v)
	}
	return t.Add(shift).In(store.ClinicLocation()).Format(store.DateFormat), nil
}

func save(m core.Maroto, name string) (string, error) {
	doc, err := m.Generate()
	if err != nil {
		return "", err
	}
	path := tmpPath(name)
	if err := doc.Save(path); err != nil {
		return "", err
	}
	return path, nil
}

func money(v float64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	whole := int64(v)
	frac := int64((v-float64(whole))*100 + 0.5)
	if frac == 100 {
		whole++
		frac = 0
	}
	intPart := fmt.Sprintf("%d", whole)
	if n := len(intPart); n > 3 {
		var b []byte
		first := n % 3
		if first > 0 {
			b = append(b, intPart[:first]...)
		}
		for i := first; i < n; i += 3 {
			if len(b) > 0 {
				b = append(b, ',')
			}
			b = append(b, intPart[i:i+3]...)
		}
		intPart = string(b)
	}
	out := fmt.Sprintf("%s.%02d", intPart, frac)
	if neg {
		out = "-" + out
	}
	return out
}

func numberToWords(n int64) string {
	if n == 0 {
		return "Zero"
	}
	if n < 0 {
		return "Negative " + numberToWords(-n)
	}
	ones := []string{"", "One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine",
		"Ten", "Eleven", "Twelve", "Thirteen", "Fourteen", "Fifteen", "Sixteen", "Seventeen", "Eighteen", "Nineteen"}
	tens := []string{"", "", "Twenty", "Thirty", "Forty", "Fifty", "Sixty", "Seventy", "Eighty", "Ninety"}

	var conv func(int64) string
	conv = func(n int64) string {
		switch {
		case n < 20:
			return ones[n]
		case n < 100:
			t := tens[n/10]
			if r := n % 10; r != 0 {
				return t + " " + ones[r]
			}
			return t
		case n < 1000:
			h := ones[n/100] + " Hundred"
			if r := n % 100; r != 0 {
				return h + " " + conv(r)
			}
			return h
		case n < 1_000_000:
			t := conv(n/1000) + " Thousand"
			if r := n % 1000; r != 0 {
				return t + " " + conv(r)
			}
			return t
		case n < 1_000_000_000:
			mi := conv(n/1_000_000) + " Million"
			if r := n % 1_000_000; r != 0 {
				return mi + " " + conv(r)
			}
			return mi
		default:
			bi := conv(n/1_000_000_000) + " Billion"
			if r := n % 1_000_000_000; r != 0 {
				return bi + " " + conv(r)
			}
			return bi
		}
	}
	return conv(n)
}
