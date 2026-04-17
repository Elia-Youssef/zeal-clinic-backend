package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllInvoices(c echo.Context) error {
	params := parseListParams(c)
	entityType := c.Param("type")
	items := models.InvoiceList{}
	total, err := items.GetAllByType(entityType, params)
	if err != nil {
		log.Println("Error: GetAllInvoices:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch invoices"})
	}
	if items == nil {
		items = []models.Invoice{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: utils.PaginatedList{Items: items, Total: total}})
}
