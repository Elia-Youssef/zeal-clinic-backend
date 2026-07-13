package csvutil

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"
)

type File struct {
	Name   string
	Header []string
	Rows   [][]string
	index  map[string]int
}

func Load(path string) (*File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	text := strings.TrimPrefix(string(raw), "\ufeff")

	r := csv.NewReader(strings.NewReader(text))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true

	var records [][]string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {

			continue
		}
		records = append(records, rec)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("%s: no rows", path)
	}

	name := path
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		name = path[i+1:]
	}

	f := &File{Name: name, Header: records[0], Rows: records[1:], index: map[string]int{}}
	for i, h := range f.Header {
		f.index[strings.TrimSpace(h)] = i
	}
	return f, nil
}

func (f *File) Col(name string) int {
	if i, ok := f.index[name]; ok {
		return i
	}
	return -1
}

func Get(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

func (f *File) Field(row []string, col string) string {
	return Get(row, f.Col(col))
}
