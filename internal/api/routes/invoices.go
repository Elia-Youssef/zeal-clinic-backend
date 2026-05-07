package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupInvoiceRoutes(api *echo.Group) {
	api.GET("/invoices", handlers.GetAllInvoices, scope("invoices:read"), cache("invoices"))
	api.GET("/invoices/:id", handlers.GetInvoiceByID, scope("invoices:read"))
	api.GET("/invoices/:id/pdf", handlers.GetInvoicePDF, scope("invoices:read"))
}
