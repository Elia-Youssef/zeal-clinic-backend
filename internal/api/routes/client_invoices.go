package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupClientInvoiceRoutes(api *echo.Group) {
	api.GET("/patients/:id/invoices", handlers.GetClientInvoices, scope("invoices:read"), cache("client-invoices"))
	// Create posts a balance transaction, records discounts, and adjusts product stock.
	// "appointments" because every appointment carries its patient's balance.
	api.POST("/client-invoices", handlers.CreateClientInvoice, criticalSync, scope("invoices:write"),
		cache("client-invoices", "invoices", "balances", "appointments", "products", "discounts", "invoice-item-discounts", "client-payments", "analytics", "reports"))
	api.PUT("/client-invoices/:id", handlers.UpdateClientInvoice, criticalSync, scope("invoices:write"), cache("client-invoices", "invoices", "appointments", "analytics", "reports"))
	api.DELETE("/client-invoices/:id", handlers.DeleteClientInvoice, criticalSync, scope("invoices:delete"),
		cache("client-invoices", "invoices", "balances", "appointments", "products", "discounts", "invoice-item-discounts", "client-payments", "analytics", "reports"))
}
