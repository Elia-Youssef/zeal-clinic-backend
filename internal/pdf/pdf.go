package pdf

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"clinic-api/internal/config"

	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/consts/border"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/props"
)

// tmpFiles tracks each generated PDF's path and creation time so the monitor
// can delete it once it ages out of the cache.
var tmpFiles = struct {
	sync.Mutex
	m map[string]time.Time
}{m: map[string]time.Time{}}

// TmpFiles returns a snapshot of tracked PDF paths and their creation times.
func TmpFiles() map[string]time.Time {
	tmpFiles.Lock()
	defer tmpFiles.Unlock()
	out := make(map[string]time.Time, len(tmpFiles.m))
	for p, t := range tmpFiles.m {
		out[p] = t
	}
	return out
}

// ForgetTmp drops a path from the tracking map; call after deleting the file.
func ForgetTmp(path string) {
	tmpFiles.Lock()
	delete(tmpFiles.m, path)
	tmpFiles.Unlock()
}

// TmpDir returns the PDF cache directory: a tmp/ subfolder next to the
// database file.
func TmpDir() string {
	dir := filepath.Join(config.DataDir(), "tmp")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

func tmpPath(name string) string {
	return filepath.Join(TmpDir(), fmt.Sprintf("%s-%d.pdf", name, time.Now().UnixNano()))
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
	tmpFiles.Lock()
	tmpFiles.m[path] = time.Now()
	tmpFiles.Unlock()
	return path, nil
}

func cellCol(size int, value string, p props.Text, bt border.Type) core.Col {
	col := text.NewCol(size, value, p)
	if bt != border.None {
		col.WithStyle(&props.Cell{BorderType: bt, BorderThickness: 0.12})
	}
	return col
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
