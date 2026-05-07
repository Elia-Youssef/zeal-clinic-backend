package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

// GetRevenueReport returns aggregated revenue at the requested tree position.
//
// Query params:
//   from, to:     YYYY-MM-DD inclusive (default: last 30 days)
//   itemKind:     "" | products | procedures (root toggle, drill mode)
//   typeId:       drill into a procedure type
//   categoryId:   drill into a procedure or product category
//   level:        "" | kind | procedure-type | procedure-category |
//                 product-category | procedure | product. When set, the
//                 report aggregates flat across the whole dataset at that
//                 level; typeId / categoryId become scope filters.
//   currencyId:   optional
func GetRevenueReport(c echo.Context) error {
	from, to := parseRange(c)
	report, err := (&store.Reports{}).Revenue(store.RevenueParams{
		From:       from,
		To:         to,
		ItemKind:   c.QueryParam("itemKind"),
		TypeID:     c.QueryParam("typeId"),
		CategoryID: c.QueryParam("categoryId"),
		Level:      c.QueryParam("level"),
		CurrencyID: c.QueryParam("currencyId"),
	})
	if err != nil {
		log.Println("Error: GetRevenueReport:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: report})
}

// GetExpensesReport returns one row per supplier-invoice line item plus one
// row per expense payment, sorted by date.
func GetExpensesReport(c echo.Context) error {
	from, to := parseRange(c)
	rows, err := (&store.Reports{}).Expenses(store.ExpensesParams{
		From:       from,
		To:         to,
		CurrencyID: c.QueryParam("currencyId"),
	})
	if err != nil {
		log.Println("Error: GetExpensesReport:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: rows})
}

// parseRange reads from/to query params, defaulting to the last 30 days (UTC)
// when either is missing, matching the analytics handler convention.
func parseRange(c echo.Context) (string, string) {
	return defaultDateRange(c.QueryParam("from"), c.QueryParam("to"), 29)
}

// defaultDateRange fills missing from/to bounds with a UTC window ending today
// and stretching `daysBack` days into the past. Empty inputs are filled; non-
// empty inputs are passed through untouched.
func defaultDateRange(from, to string, daysBack int) (string, string) {
	if to == "" {
		to = string(store.DateToday())
	}
	if from == "" {
		from = string(store.DateOffsetDays(-daysBack))
	}
	return from, to
}
