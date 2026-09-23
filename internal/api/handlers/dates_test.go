package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"clinic-api/internal/database/store"

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

// parseRange normalizes both accepted forms to RFC3339 UTC instants and
// answers the default clinic-local window when a bound is missing.
func TestParseRange(t *testing.T) {
	rangeOf := func(query string) (string, string, error) {
		req := httptest.NewRequest(http.MethodGet, "/?"+query, nil)
		return parseRange(echo.New().NewContext(req, httptest.NewRecorder()))
	}
	for _, tc := range []struct{ name, query, wantFrom, wantTo string }{
		// Bare dates mean the clinic-local calendar days (UTC+3 in May 2026).
		{"bare dates", "from=2026-05-01&to=2026-05-31", "2026-04-30T21:00:00Z", "2026-05-31T21:00:00Z"},
		{"UTC instants kept as given", "from=2026-04-30T21:00:00Z&to=2026-05-31T21:00:00Z", "2026-04-30T21:00:00Z", "2026-05-31T21:00:00Z"},
		{"an offset instant normalizes to UTC", "from=2026-05-01T00:00:00%2B03:00&to=2026-06-01T00:00:00%2B03:00", "2026-04-30T21:00:00Z", "2026-05-31T21:00:00Z"},
	} {
		from, to, err := rangeOf(tc.query)
		if err != nil || from != tc.wantFrom || to != tc.wantTo {
			t.Errorf("%s: parseRange(%q) = %q, %q, %v; want %q, %q, no error", tc.name, tc.query, from, to, err, tc.wantFrom, tc.wantTo)
		}
	}
	for _, query := range []string{"from=yesterday", "to=2026-05", "from=2026-05-01T00:00:00", "to=2026-05-01%2000:00:00"} {
		if _, _, err := rangeOf(query); err == nil {
			t.Errorf("parseRange(%q) accepted a malformed bound", query)
		}
	}
	// A missing bound defaults to the clinic-local window of the last 30 days.
	wantFrom, wantTo := store.ClinicRangeLastNDays(29)
	if from, to, err := rangeOf(""); err != nil || from != string(wantFrom) || to != string(wantTo) {
		t.Errorf("parseRange() = %q, %q, %v; want the default window %q, %q", from, to, err, wantFrom, wantTo)
	}
}

// The handlers whose date is required or whose range is strictly parsed
// refuse a malformed value with 400 before they touch the database (none is
// open here).
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
		{"appointment list", "date=31-03-2025", GetAllAppointments, "Invalid date"},
		{"appointment list without a date", "", GetAllAppointments, "Date is required"},
		{"count per room", "date=31-03-2025", GetAppointmentCountPerRoom, "Invalid date"},
		{"count per room without a date", "", GetAppointmentCountPerRoom, "Date is required"},
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
