package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupSupplierInvoiceRoutes(api *echo.Group) {
	api.GET("/suppliers/:id/invoices", handlers.GetSupplierInvoices, scope("invoices:read"), cache("supplier-invoices"))
	// Create posts a balance transaction and increments product stock.
	api.POST("/supplier-invoices", handlers.CreateSupplierInvoice, scope("invoices:write"),
		cache("supplier-invoices", "invoices", "balances", "products", "supplier-payments", "analytics"))
	api.PUT("/supplier-invoices/:id", handlers.UpdateSupplierInvoice, scope("invoices:write"), cache("supplier-invoices", "invoices", "analytics"))
	api.PUT("/supplier-invoices/:id/items/:itemId", handlers.UpdateSupplierInvoiceItem, scope("invoices:write"),
		cache("supplier-invoices", "invoices", "balances", "supplier-payments", "analytics"))
	api.DELETE("/supplier-invoices/:id", handlers.DeleteSupplierInvoice, scope("invoices:delete"),
		cache("supplier-invoices", "invoices", "balances", "products", "supplier-payments", "analytics"))
}
