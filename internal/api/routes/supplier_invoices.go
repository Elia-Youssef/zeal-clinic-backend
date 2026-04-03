package routes

import (
	"clinic-api/internal/api/handlers"
	"clinic-api/internal/api/middleware"

	"github.com/labstack/echo/v4"
)

func SetupSupplierInvoiceRoutes(api *echo.Group) {
	api.GET("/suppliers/:id/invoices", handlers.GetSupplierInvoices, scope("transactions:read"), middleware.CacheMiddleware("supplier-invoices"))
	api.GET("/supplier-invoices/:id", handlers.GetSupplierInvoiceByID, scope("transactions:read"), middleware.CacheMiddleware("supplier-invoices"))
	api.POST("/supplier-invoices", handlers.CreateSupplierInvoice, scope("transactions:write"), middleware.CacheMiddleware("supplier-invoices"))
	api.PUT("/supplier-invoices/:id", handlers.UpdateSupplierInvoice, scope("transactions:write"), middleware.CacheMiddleware("supplier-invoices"))
	api.DELETE("/supplier-invoices/:id", handlers.DeleteSupplierInvoice, scope("transactions:delete"), middleware.CacheMiddleware("supplier-invoices"))
}
