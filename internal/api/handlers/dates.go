package handlers

import (
	"errors"
	"net/http"
	"time"

	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"

	"github.com/labstack/echo/v4"
)

var (
	errDateRequired = errors.New("date is required")
	errInvalidDate  = errors.New("invalid date")
)

// parseDate reads a calendar-day parameter: YYYY-MM-DD and nothing else.
func parseDate(value string) (time.Time, error) {
	day, err := time.Parse(store.DateFormat, value)
	if err != nil {
		return time.Time{}, errInvalidDate
	}
	return day, nil
}

// requiredDate reads the date query parameter: a day the caller must be
// given, a YYYY-MM-DD and nothing else.
func requiredDate(c echo.Context) (string, error) {
	value := c.QueryParam("date")
	if value == "" {
		return "", errDateRequired
	}
	day, err := parseDate(value)
	if err != nil {
		return "", err
	}
	return day.Format(store.DateFormat), nil
}

// parseRange reads the from and to query params. Each one is an RFC3339
// instant (from inclusive, to exclusive; the frontend converts clinic-local
// boundaries to UTC) or a bare YYYY-MM-DD (the clinic-local calendar day it
// names, inclusive on both ends); the store's parser normalizes both forms
// to RFC3339 UTC instants. A missing one defaults to a clinic-local window
// of the last 30 days.
func parseRange(c echo.Context) (from, to string, err error) {
	if from, err = store.ParseRangeStart(c.QueryParam("from")); err != nil {
		return "", "", errInvalidDate
	}
	if to, err = store.ParseRangeEnd(c.QueryParam("to")); err != nil {
		return "", "", errInvalidDate
	}
	from, to = defaultDateRange(from, to, 29)
	return from, to, nil
}

// defaultDateRange fills missing from/to bounds with a clinic-local window
// ending at end-of-today and reaching `daysBack` days into the past, expressed
// as RFC3339 UTC instants like the parsed ones.
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

// invalidDate answers a missing or malformed date parameter with its own
// message.
func invalidDate(c echo.Context, err error) error {
	message := "Invalid date"
	if errors.Is(err, errDateRequired) {
		message = "Date is required"
	}
	return c.JSON(http.StatusBadRequest, httpx.Response{Error: message})
}

// invalidDateRange answers a malformed from or to parameter.
func invalidDateRange(c echo.Context) error {
	return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid date range"})
}
