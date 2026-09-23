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
//
//	from, to:     date-range bounds. Accepts either RFC3339 UTC instants
//	              (from inclusive, to exclusive; the frontend converts
//	              clinic-local boundaries to UTC) or bare YYYY-MM-DD dates
//	              (the clinic-local calendar days they name, inclusive on
//	              both ends); a missing bound defaults to a clinic-local
//	              window of the last 30 days.
//	itemKind:     "" | products | procedures | other | discounts | all
//	              (root toggle, drill mode). "discounts" reports invoice
//	              offers as negative amounts (revenue given up rather than
//	              earned), so it never mixes into the other kinds or "all".
//	typeId:       drill into a procedure type
//	categoryId:   drill into a procedure or product category
//	level:        "" | kind | procedure-type | procedure-category |
//	              product-category | procedure | product | other | discount.
//	              When set, the report aggregates flat across the whole
//	              dataset at that level; typeId / categoryId become scope
//	              filters.
func GetRevenueReport(c echo.Context) error {
	from, to, err := parseRange(c)
	if err != nil {
		return invalidDateRange(c)
	}
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
	from, to, err := parseRange(c)
	if err != nil {
		return invalidDateRange(c)
	}
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
