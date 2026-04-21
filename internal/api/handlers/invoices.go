package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllInvoices(c echo.Context) error {
	params := parseListParams(c)
	entityType := c.Param("type")
	items := store.InvoiceList{}
	total, err := items.GetAllByType(entityType, params)
	if err != nil {
		log.Println("Error: GetAllInvoices:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch invoices"})
	}
	if items == nil {
		items = []store.Invoice{}
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: items, Total: total}})
}
