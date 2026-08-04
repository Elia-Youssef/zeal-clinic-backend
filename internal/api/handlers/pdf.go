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
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "Invoice not found"})
	} else if err != nil {
		log.Println("Error: GetInvoicePDF:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load invoice"})
	}
	path, err := pdf.GenerateInvoice(&inv)
	if err != nil {
		log.Println("Error: GenerateInvoice:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't generate PDF"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: map[string]string{"url": "/files/" + filepath.Base(path)}})
}

// GetAllAppointmentsPDF generates a PDF listing appointments. Mirrors
// GetAllAppointments: requires a `date` query param and honors the same list
// params (filter, sort, order). When `range=week` is passed it lists the whole
// Monday-Sunday week containing date instead of the single day. Cancelled
// appointments are listed too, marked as such.
func GetAllAppointmentsPDF(c echo.Context) error {
	date := c.QueryParam("date")
	if date == "" {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Date is required"})
	}
	params := parseListParams(c)
	apts := store.AppointmentList{}
	var err error
	if c.QueryParam("range") == "week" {
		_, err = apts.GetWeek(date, params)
	} else {
		_, err = apts.GetAll(date, params)
	}
	if err != nil {
		log.Println("Error: GetAllAppointmentsPDF failed to fetch appointments:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load appointments"})
	}

	dropdown, err := store.GetRoomDropdown(store.ListParams{})
	if err != nil {
		log.Println("Error: GetAllAppointmentsPDF failed to fetch rooms:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load rooms"})
	}
	rooms := make(map[string]string, len(dropdown))
	for _, r := range dropdown {
		rooms[r.ID] = r.Name
	}

	path, err := pdf.GenerateAppointments(apts, rooms, date)
	if err != nil {
		log.Println("Error: GenerateAppointments:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't generate PDF"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: map[string]string{"url": "/files/" + filepath.Base(path)}})
}

// GetRevenueReportPDF generates a PDF for the revenue report.
func GetRevenueReportPDF(c echo.Context) error {
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
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Couldn't load report"})
	}
	path, err := pdf.GenerateRevenueReport(&report, from, to)
	if err != nil {
		log.Println("Error: GenerateRevenueReport:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't generate PDF"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: map[string]string{"url": "/files/" + filepath.Base(path)}})
}

// GetAnalyticsReportPDF generates the dashboard-style analytics PDF for the
// requested date range.
func GetAnalyticsReportPDF(c echo.Context) error {
	from, to := parseRange(c)
	data, err := (&store.Analytics{}).Report(from, to)
	if err != nil {
		log.Println("Error: GetAnalyticsReportPDF:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load analytics"})
	}
	path, err := pdf.GenerateAnalyticsReport(&data, from, to)
	if err != nil {
		log.Println("Error: GenerateAnalyticsReport:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't generate PDF"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: map[string]string{"url": "/files/" + filepath.Base(path)}})
}

// GetExpensesReportPDF generates a PDF for the expenses report.
func GetExpensesReportPDF(c echo.Context) error {
	from, to := parseRange(c)
	report, err := (&store.Reports{}).Expenses(store.ExpensesParams{
		From:       from,
		To:         to,
		CurrencyID: store.USDCurrencyID,
	})
	if err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Couldn't load report"})
	}
	path, err := pdf.GenerateExpensesReport(report, from, to)
	if err != nil {
		log.Println("Error: GenerateExpensesReport:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't generate PDF"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: map[string]string{"url": "/files/" + filepath.Base(path)}})
}
