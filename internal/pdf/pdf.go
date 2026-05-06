package pdf

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/consts/border"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/props"
)

func TmpDir() string {
	base := "."
	if exe, err := os.Executable(); err == nil {
		base = filepath.Dir(exe)
	}
	dir := filepath.Join(base, "tmp")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

func tmpPath(name string) string {
	return filepath.Join(TmpDir(), fmt.Sprintf("%s.pdf", name))
}

// money formats a float as a string with comma thousand-separators and two
// decimal places, e.g. 143200000 becomes "143,200,000.00".
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

// numberToWords renders a non-negative integer as English words, e.g.
// 1600 becomes "One Thousand Six Hundred". Used on invoices.
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

func newCol(size int, value string, p props.Text, borders ...bool) core.Col {
	borderStyle := &props.Cell{BorderType: border.None, BorderThickness: 0.1}
	col := text.NewCol(size, value, p)
	for i, b := range borders {
		if b {
			switch i {
			case 0:
				borderStyle.BorderType |= border.Top
			case 1:
				borderStyle.BorderType |= border.Right
			case 2:
				borderStyle.BorderType |= border.Bottom
			case 3:
				borderStyle.BorderType |= border.Left
			}
			col.WithStyle(borderStyle)
		}
	}
	return col
}
