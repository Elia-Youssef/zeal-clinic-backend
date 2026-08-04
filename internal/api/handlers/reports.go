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
//   from, to:     date-range bounds (default: last 30 UTC days). Accepts
//                 either RFC3339 UTC instants (from inclusive, to exclusive;
//                 the frontend converts clinic-local boundaries to UTC) or
//                 bare YYYY-MM-DD UTC dates (calendar-day inclusive on both
//                 ends).
//   itemKind:     "" | products | procedures | other | discounts | all
//                 (root toggle, drill mode). "discounts" reports invoice
//                 offers as negative amounts (revenue given up rather than
//                 earned), so it never mixes into the other kinds or "all".
//   typeId:       drill into a procedure type
//   categoryId:   drill into a procedure or product category
//   level:        "" | kind | procedure-type | procedure-category |
//                 product-category | procedure | product | other | discount.
//                 When set, the report aggregates flat across the whole
//                 dataset at that level; typeId / categoryId become scope
//                 filters.
func GetRevenueReport(c echo.Context) error {
	from, to := parseRange(c)
	report, err := (&store.Reports{}).Revenue(store.RevenueParams{
		From:       from,
		To:         to,
		ItemKind:   c.QueryParam("itemKind"),
		TypeID:     c.QueryParam("typeId"),
		CategoryID: c.QueryParam("categoryId"),
		Level:      c.QueryParam("level"),
		CurrencyID: store.USDCurrencyID,
	})
	if err != nil {
		log.Println("Error: GetRevenueReport:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Couldn't load report"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: report})
}

// GetExpensesReport returns one row per supplier-invoice line item plus one
// row per expense payment, sorted by date.
func GetExpensesReport(c echo.Context) error {
	from, to := parseRange(c)
	report, err := (&store.Reports{}).Expenses(store.ExpensesParams{
		From:       from,
		To:         to,
		CurrencyID: store.USDCurrencyID,
	})
	if err != nil {
		log.Println("Error: GetExpensesReport:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Couldn't load report"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: report})
}

// parseRange reads from/to query params, defaulting to the last 30 days (UTC)
// when either is missing, matching the analytics handler convention.
func parseRange(c echo.Context) (string, string) {
	return defaultDateRange(c.QueryParam("from"), c.QueryParam("to"), 29)
}

// defaultDateRange fills missing from/to bounds with a clinic-local window
// ending at end-of-today and reaching `daysBack` days into the past, expressed
// as RFC3339 UTC instants so they pass through RangeStart/RangeEnd unchanged.
// Non-empty inputs are forwarded untouched and re-normalized at the store
// layer (accepts either RFC3339 or bare YYYY-MM-DD).
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
