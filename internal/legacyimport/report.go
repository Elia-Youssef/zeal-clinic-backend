package legacyimport

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type Report struct {
	sections []*section
	issues   []string
}

type section struct {
	title string
	lines []string
	read  int
	wrote int
	skip  int
}

func newReport() *Report { return &Report{} }

func (r *Report) Section(title string) *section {
	for _, s := range r.sections {
		if s.title == title {
			return s
		}
	}
	s := &section{title: title}
	r.sections = append(r.sections, s)
	return s
}

func (s *section) Counts(read, wrote, skip int) { s.read, s.wrote, s.skip = read, wrote, skip }

func (s *section) Note(format string, a ...any) {
	s.lines = append(s.lines, "  - "+fmt.Sprintf(format, a...))
}

func (s *section) Issue(r *Report, format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	s.lines = append(s.lines, "  ! "+msg)
	r.issues = append(r.issues, fmt.Sprintf("[%s] %s", s.title, msg))
}

func (r *Report) WriteFile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)

	fmt.Fprintln(w, "CLINIC DATA MIGRATION REPORT")
	fmt.Fprintln(w, "============================")
	fmt.Fprintln(w, "Generated while importing old_data/*.csv directly into the clinic database.")
	fmt.Fprintln(w)

	fmt.Fprintln(w, "ROW COUNTS PER TABLE")
	fmt.Fprintln(w, "--------------------")
	fmt.Fprintf(w, "%-26s %8s %8s %8s\n", "table/source", "read", "written", "skipped")
	for _, s := range r.sections {
		fmt.Fprintf(w, "%-26s %8d %8d %8d\n", s.title, s.read, s.wrote, s.skip)
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "INVALID / ADJUSTED VALUES (summary)")
	fmt.Fprintln(w, "-----------------------------------")
	if len(r.issues) == 0 {
		fmt.Fprintln(w, "  none")
	}
	for _, is := range r.issues {
		fmt.Fprintf(w, "  - %s\n", is)
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "DETAIL BY TABLE")
	fmt.Fprintln(w, "---------------")
	for _, s := range r.sections {
		fmt.Fprintf(w, "\n### %s  (read %d, written %d, skipped %d)\n", s.title, s.read, s.wrote, s.skip)
		if len(s.lines) == 0 {
			fmt.Fprintln(w, "  (no notes)")
		}
		for _, l := range s.lines {
			fmt.Fprintln(w, l)
		}
	}
	return w.Flush()
}

func (r *Report) PrintSummary() {
	fmt.Println(strings.Repeat("-", 60))
	fmt.Printf("%-26s %8s %8s %8s\n", "table/source", "read", "written", "skipped")
	for _, s := range r.sections {
		fmt.Printf("%-26s %8d %8d %8d\n", s.title, s.read, s.wrote, s.skip)
	}
	fmt.Println(strings.Repeat("-", 60))
	fmt.Printf("Invalid / adjusted values (%d):\n", len(r.issues))
	for _, is := range r.issues {
		fmt.Printf("  - %s\n", is)
	}
	fmt.Println(strings.Repeat("-", 60))
}
