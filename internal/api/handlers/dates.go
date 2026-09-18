package handlers

import (
	"errors"
	"net/http"
	"time"

	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"

	"github.com/labstack/echo/v4"
)

var errInvalidDate = errors.New("invalid date")

// parseDate reads a calendar-day parameter: YYYY-MM-DD and nothing else.
func parseDate(value string) (time.Time, error) {
	day, err := time.Parse(store.DateFormat, value)
	if err != nil {
		return time.Time{}, errInvalidDate
	}
	return day, nil
}

// rangeBound checks a from or to parameter: an RFC3339 instant or a bare
// YYYY-MM-DD, the two forms the store's RangeStart and RangeEnd accept. The
// value comes back as given; an empty one stays empty for the default.
func rangeBound(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if len(value) == len(store.DateFormat) {
		if _, err := time.Parse(store.DateFormat, value); err == nil {
			return value, nil
		}
		return "", errInvalidDate
	}
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return "", errInvalidDate
	}
	return value, nil
}

// parseRange reads the from and to query params. Each one is an RFC3339 UTC
// instant (from inclusive, to exclusive; the frontend converts clinic-local
// boundaries to UTC) or a bare YYYY-MM-DD (calendar-day inclusive on both
// ends); a missing one defaults to a clinic-local window of the last 30 days.
func parseRange(c echo.Context) (from, to string, err error) {
	if from, err = rangeBound(c.QueryParam("from")); err != nil {
		return "", "", err
	}
	if to, err = rangeBound(c.QueryParam("to")); err != nil {
		return "", "", err
	}
	from, to = defaultDateRange(from, to, 29)
	return from, to, nil
}

// defaultDateRange fills missing from/to bounds with a clinic-local window
// ending at end-of-today and reaching `daysBack` days into the past, expressed
// as RFC3339 UTC instants so they pass through RangeStart/RangeEnd unchanged.
// Non-empty inputs are forwarded untouched and re-normalized at the store
// layer.
func defaultDateRange(from, to string, daysBack int) (string, string) {
	if from == "" || to == "" {
		dStart, dEnd := store.ClinicRangeLastNDays(daysBack)
		if from == "" {
			from = string(dStart)
		}
		if to == "" {
			to = string(dEnd)
		}
	}
	return from, to
}

// invalidDateRange answers a malformed from or to parameter.
func invalidDateRange(c echo.Context) error {
	return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid date range"})
}
