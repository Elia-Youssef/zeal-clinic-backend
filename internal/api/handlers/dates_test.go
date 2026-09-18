package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestParseDate(t *testing.T) {
	for value, ok := range map[string]bool{
		"2026-05-20":           true,
		"2024-02-29":           true,
		"":                     false,
		"20-05-2026":           false,
		"2026-5-20":            false,
		"2026-02-30":           false,
		"2026-05-20T00:00:00Z": false,
		"2026-05-20x":          false,
		"../2026-05-20":        false,
	} {
		if _, err := parseDate(value); (err == nil) != ok {
			t.Errorf("parseDate(%q): error %v, want accepted %v", value, err, ok)
		}
	}
}

func TestRangeBound(t *testing.T) {
	for value, ok := range map[string]bool{
		"":                          true,
		"2026-05-01":                true,
		"2026-05-01T00:00:00Z":      true,
		"2026-04-30T21:00:00.000Z":  true,
		"2026-04-30T21:00:00+03:00": true,
		"2026":                      false,
		"2026-05":                   false,
		"20-05-2026":                false,
		"2026-05-01 00:00:00":       false,
		"2026-05-01T00:00:00":       false,
		"2026-05-01T25:00:00Z":      false,
		"bogus":                     false,
		"2026-05-01/../x":           false,
	} {
		got, err := rangeBound(value)
		if (err == nil) != ok || (ok && got != value) {
			t.Errorf("rangeBound(%q) = %q, %v; want accepted %v", value, got, err, ok)
		}
	}
}

// Every handler with a date parameter refuses a malformed value with 400
// before it touches the database (none is open here).
func TestHandlersRefuseMalformedDates(t *testing.T) {
	for _, tc := range []struct {
		name, query string
		handler     echo.HandlerFunc
		message     string
	}{
		{"schedule PDF", "date=31-03-2025", GetAllAppointmentsPDF, "Invalid date"},
		{"schedule PDF with a path in the date", "date=..%2F..%2F2025-03-31", GetAllAppointmentsPDF, "Invalid date"},
		{"schedule PDF without a date", "", GetAllAppointmentsPDF, "Date is required"},
		{"revenue report", "from=2026", GetRevenueReport, "Invalid date range"},
		{"expenses report", "to=bogus", GetExpensesReport, "Invalid date range"},
		{"revenue report PDF", "from=2026-13-01", GetRevenueReportPDF, "Invalid date range"},
		{"expenses report PDF", "from=2026-05-01&to=2026-05", GetExpensesReportPDF, "Invalid date range"},
		{"analytics report PDF", "to=2026-05-01T25:00:00Z", GetAnalyticsReportPDF, "Invalid date range"},
		{"analytics money", "from=2026", GetAnalyticsMoney, "Invalid date range"},
		{"analytics patients", "to=yesterday", GetAnalyticsPatients, "Invalid date range"},
		{"analytics series", "metric=revenue&from=yesterday", GetAnalyticsSeries, "Invalid date range"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/?"+tc.query, nil)
			rec := httptest.NewRecorder()
			if err := tc.handler(echo.New().NewContext(req, rec)); err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"Error":"`+tc.message+`"`) {
				t.Errorf("%d %s, want 400 with %q", rec.Code, rec.Body.String(), tc.message)
			}
		})
	}
}
