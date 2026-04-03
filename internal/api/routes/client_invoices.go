package routes

import (
	"clinic-api/internal/api/handlers"
	"clinic-api/internal/api/middleware"

	"github.com/labstack/echo/v4"
)

func SetupClientInvoiceRoutes(api *echo.Group) {
	api.GET("/patients/:id/invoices", handlers.GetClientInvoices, scope("transactions:read"), middleware.CacheMiddleware("client-invoices"))
	api.GET("/client-invoices/:id", handlers.GetClientInvoiceByID, scope("transactions:read"), middleware.CacheMiddleware("client-invoices"))
	api.POST("/client-invoices", handlers.CreateClientInvoice, scope("transactions:write"), middleware.CacheMiddleware("client-invoices"))
	api.PUT("/client-invoices/:id", handlers.UpdateClientInvoice, scope("transactions:write"), middleware.CacheMiddleware("client-invoices"))
	api.DELETE("/client-invoices/:id", handlers.DeleteClientInvoice, scope("transactions:delete"), middleware.CacheMiddleware("client-invoices"))
}
