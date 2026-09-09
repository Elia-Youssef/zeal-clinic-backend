package pdf

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"clinic-api/internal/database/store"
)

// pdfStructure reads the parts of a generated PDF that don't depend on its
// compressed content: header, trailer and page tree.
func pdfStructure(t *testing.T, path string) (pages, pageObjects int) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(raw, []byte("%PDF-1.")) {
		t.Fatalf("%s: missing %%PDF- header", filepath.Base(path))
	}
	if !bytes.HasSuffix(bytes.TrimRight(raw, "\r\n"), []byte("\n%%EOF")) {
		t.Fatalf("%s: does not end with %%%%EOF", filepath.Base(path))
	}
	if !bytes.Contains(raw, []byte("\ntrailer\n")) || !bytes.Contains(raw, []byte("\nstartxref\n")) {
		t.Fatalf("%s: no trailer or startxref", filepath.Base(path))
	}
	lines := bytes.Split(raw, []byte("\n"))
	pages = -1
	for i, line := range lines {
		switch string(line) {
		case "<</Type /Page":
			pageObjects++
		case "<</Type /Pages":
			for _, next := range lines[i+1:] {
				if n, ok := bytes.CutPrefix(next, []byte("/Count ")); ok {
					if pages, err = strconv.Atoi(string(n)); err != nil {
						t.Fatalf("%s: page count %q", filepath.Base(path), n)
					}
					break
				}
			}
		}
	}
	return pages, pageObjects
}

func sampleInvoice() *store.Invoice {
	return &store.Invoice{
		ID: "inv1", InvoiceNumber: 42, ToEntityName: "Test Patient",
		Amount: 300, DiscountValue: 50, FinalAmount: 250, CreatedAt: store.Date("2026-05-20"),
		Items: []store.InvoiceItem{
			{ID: "i1", ItemType: "procedure", ItemID: "p1", Quantity: 1, Amount: 200, FinalAmount: 200, ItemName: "Sample Procedure"},
			{ID: "i2", ItemType: "product", ItemID: "pr1", Quantity: 2, Amount: 100, FinalAmount: 100, ItemName: "Sample Serum"},
		},
	}
}

func sampleRevenue() *store.RevenueReport {
	return &store.RevenueReport{
		Level:  "procedure",
		Totals: store.RevenueTotals{Quantity: 3, Amount: 770},
		Items: []store.RevenueGroup{
			{EntityName: "Sample Procedure", Quantity: 2, Amount: 440, Percentage: 57.1},
			{EntityName: "Other Procedure", Quantity: 1, Amount: 330, Percentage: 42.9},
		},
	}
}

func sampleExpenses() store.ExpensesReport {
	return store.ExpensesReport{
		Rows: []store.ExpenseRow{
			{Date: store.Date("2026-05-10"), Supplier: "Sample Supplier", Description: "Stock", Quantity: 4, Amount: 840},
		},
		Totals: store.ExpensesTotals{Amount: 840},
	}
}

func sampleAppointments(n int) store.AppointmentList {
	var list store.AppointmentList
	for i := 0; i < n; i++ {
		list = append(list, store.Appointment{
			PatientName: fmt.Sprintf("Patient %d", i), RoomID: "r1", Status: "Completed",
			StartTime: store.Date("2026-05-20T10:00:00Z"), EndTime: store.Date("2026-05-20T11:00:00Z"),
			AppointmentProcedures: []store.AppointmentProcedure{{ProcedureName: "Sample Procedure"}},
		})
	}
	return list
}

// Each generator writes <name>-<unix milliseconds>.pdf into the PDF cache
// folder and returns that path.
func TestGenerators_WriteNamedPDFsIntoTheCache(t *testing.T) {
	t.Chdir(t.TempDir())
	rooms := map[string]string{"r1": "Room 1"}
	cases := []struct {
		name    string
		render  func() (string, error)
		pattern string
		pages   int
	}{
		{"invoice", func() (string, error) { return GenerateInvoice(sampleInvoice()) },
			`^invoice-42-\d{13}\.pdf$`, 1},
		{"revenue report", func() (string, error) { return GenerateRevenueReport(sampleRevenue(), "2026-05-01", "2026-05-31") },
			`^revenue-report-2026-05-01-2026-05-31-\d{13}\.pdf$`, 1},
		// RFC3339 bounds are cut to their first ten characters: the UTC date.
		{"revenue report, clinic-local bounds", func() (string, error) {
			return GenerateRevenueReport(sampleRevenue(), "2026-04-30T21:00:00Z", "2026-05-31T21:00:00Z")
		}, `^revenue-report-2026-04-30-2026-05-31-\d{13}\.pdf$`, 1},
		{"expenses report", func() (string, error) { return GenerateExpensesReport(sampleExpenses(), "2026-05-01", "2026-05-31") },
			`^expenses-report-2026-05-01-2026-05-31-\d{13}\.pdf$`, 1},
		{"appointments", func() (string, error) { return GenerateAppointments(sampleAppointments(2), rooms, "2026-05-20") },
			`^appointments-2026-05-20-\d{13}\.pdf$`, 1},
		{"appointments, many rows", func() (string, error) { return GenerateAppointments(sampleAppointments(120), rooms, "2026-05-20") },
			`^appointments-2026-05-20-\d{13}\.pdf$`, 6},
		{"analytics report", func() (string, error) {
			return GenerateAnalyticsReport(&store.AnalyticsReport{}, "2026-05-01T00:00:00Z", "2026-05-31T00:00:00Z")
		}, `^analytics-report-2026-05-01-2026-05-31-\d{13}\.pdf$`, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path, err := tc.render()
			if err != nil {
				t.Fatal(err)
			}
			if filepath.Dir(path) != TmpDir() {
				t.Errorf("written to %s, want the cache folder %s", filepath.Dir(path), TmpDir())
			}
			if !regexp.MustCompile(tc.pattern).MatchString(filepath.Base(path)) {
				t.Errorf("file name %s does not match %s", filepath.Base(path), tc.pattern)
			}
			pages, objects := pdfStructure(t, path)
			if pages != objects {
				t.Errorf("page tree counts %d pages but has %d page objects", pages, objects)
			}
			if pages != tc.pages {
				t.Errorf("pages = %d, want %d", pages, tc.pages)
			}
		})
	}
}

func TestGenerators_RejectEmptyInput(t *testing.T) {
	t.Chdir(t.TempDir())
	if _, err := GenerateInvoice(nil); err == nil || err.Error() != "invoice is empty" {
		t.Errorf("nil invoice: %v", err)
	}
	if _, err := GenerateInvoice(&store.Invoice{}); err == nil || err.Error() != "invoice is empty" {
		t.Errorf("invoice without id: %v", err)
	}
	if _, err := GenerateAnalyticsReport(nil, "2026-05-01", "2026-05-31"); err == nil || err.Error() != "report is empty" {
		t.Errorf("nil analytics report: %v", err)
	}
}

// Report bounds shorter than a date make the generators panic while naming
// the file.
func TestGenerators_PanicOnShortRangeBounds(t *testing.T) {
	t.Chdir(t.TempDir())
	for name, render := range map[string]func(){
		"revenue":   func() { _, _ = GenerateRevenueReport(sampleRevenue(), "2026-05", "2026-05-31") },
		"expenses":  func() { _, _ = GenerateExpensesReport(sampleExpenses(), "2026-05-01", "2026-05") },
		"analytics": func() { _, _ = GenerateAnalyticsReport(&store.AnalyticsReport{}, "", "2026-05-31") },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: no panic on a short bound", name)
				}
			}()
			render()
		}()
	}
	if entries, _ := os.ReadDir(TmpDir()); len(entries) != 0 {
		t.Errorf("files written before the panic: %d", len(entries))
	}
}

func TestMoneyAndWords(t *testing.T) {
	for v, want := range map[float64]string{
		0: "0.00", 5: "5.00", -5: "-5.00", 0.29: "0.29", 0.995: "1.00", 1234.5: "1,234.50",
		999.999: "1,000.00", 1000: "1,000.00", 1234567.891: "1,234,567.89", -1234.5: "-1,234.50",
	} {
		if got := money(v); got != want {
			t.Errorf("money(%v) = %q, want %q", v, got, want)
		}
	}
	for n, want := range map[int64]string{
		0: "Zero", 7: "Seven", 15: "Fifteen", 42: "Forty Two", 100: "One Hundred", 180: "One Hundred Eighty",
		1001: "One Thousand One", 1234567: "One Million Two Hundred Thirty Four Thousand Five Hundred Sixty Seven",
		-5: "Negative Five", 2000000000: "Two Billion",
	} {
		if got := numberToWords(n); got != want {
			t.Errorf("numberToWords(%d) = %q, want %q", n, got, want)
		}
	}
	for in, want := range map[store.Date]string{
		"2026-05-20T22:30:00Z": "21/05/2026",
		"2026-05-20":           "20/05/2026",
		"garbage":              "garbage",
	} {
		if got := clinicDate(in); got != want {
			t.Errorf("clinicDate(%q) = %q, want %q", in, got, want)
		}
	}
}
