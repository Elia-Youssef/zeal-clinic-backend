package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupClientInvoiceRoutes(api *echo.Group) {
	api.GET("/patients/:id/invoices", handlers.GetClientInvoices, scope("transactions:read"), cache("client-invoices"))
	// Create posts a balance transaction, records discounts, and adjusts product stock.
	api.POST("/client-invoices", handlers.CreateClientInvoice, scope("transactions:write"),
		cache("client-invoices", "invoices", "balances", "products", "discounts", "invoice-item-discounts", "client-payments", "analytics"))
	api.PUT("/client-invoices/:id", handlers.UpdateClientInvoice, scope("transactions:write"), cache("client-invoices", "invoices", "analytics"))
	api.DELETE("/client-invoices/:id", handlers.DeleteClientInvoice, scope("transactions:delete"), cache("client-invoices", "invoices", "balances", "analytics"))
}
