package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupSupplierInvoiceRoutes(api *echo.Group) {
	api.GET("/supplier-invoices", handlers.GetSupplierInvoices, scope("transactions:read"))
	api.GET("/supplier-invoices/:id", handlers.GetSupplierInvoiceByID, scope("transactions:read"))
	api.POST("/supplier-invoices", handlers.CreateSupplierInvoice, scope("transactions:write"))
	api.PUT("/supplier-invoices/:id", handlers.UpdateSupplierInvoice, scope("transactions:write"))
	api.DELETE("/supplier-invoices/:id", handlers.DeleteSupplierInvoice, scope("transactions:delete"))
}
