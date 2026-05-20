package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"clinic-api/internal/pdf"
	"errors"
	"log"
	"net/http"
	"path/filepath"

	"github.com/labstack/echo/v4"
)

// GetInvoicePDF generates a PDF for the invoice and returns its served URL.
func GetInvoicePDF(c echo.Context) error {
	var inv store.Invoice
	if err := inv.GetByID(c.Param("id")); errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "invoice not found"})
	} else if err != nil {
		log.Println("Error: GetInvoicePDF:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch invoice"})
	}
	path, err := pdf.GenerateInvoice(&inv)
	if err != nil {
		log.Println("Error: GenerateInvoice:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to generate pdf"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: map[string]string{"url": "/files/" + filepath.Base(path)}})
}

// GetRevenueReportPDF generates a PDF for the revenue report.
func GetRevenueReportPDF(c echo.Context) error {
	from, to := parseRange(c)
	currencyID := c.QueryParam("currencyId")
	report, err := (&store.Reports{}).Revenue(store.RevenueParams{
		From:       from,
		To:         to,
		ItemKind:   c.QueryParam("itemKind"),
		TypeID:     c.QueryParam("typeId"),
		CategoryID: c.QueryParam("categoryId"),
		Level:      c.QueryParam("level"),
		CurrencyID: currencyID,
	})
	if err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: err.Error()})
	}
	path, err := pdf.GenerateRevenueReport(&report, from, to)
	if err != nil {
		log.Println("Error: GenerateRevenueReport:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to generate pdf"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: map[string]string{"url": "/files/" + filepath.Base(path)}})
}

// GetExpensesReportPDF generates a PDF for the expenses report.
func GetExpensesReportPDF(c echo.Context) error {
	from, to := parseRange(c)
	currencyID := c.QueryParam("currencyId")
	rows, err := (&store.Reports{}).Expenses(store.ExpensesParams{
		From:       from,
		To:         to,
		CurrencyID: currencyID,
	})
	if err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: err.Error()})
	}
	path, err := pdf.GenerateExpensesReport(rows, from, to)
	if err != nil {
		log.Println("Error: GenerateExpensesReport:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to generate pdf"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: map[string]string{"url": "/files/" + filepath.Base(path)}})
}
