package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupClientInvoiceRoutes(api *echo.Group) {
	api.GET("/client-invoices", handlers.GetClientInvoices, scope("transactions:read"))
	api.GET("/client-invoices/:id", handlers.GetClientInvoiceByID, scope("transactions:read"))
	api.POST("/client-invoices", handlers.CreateClientInvoice, scope("transactions:write"))
	api.PUT("/client-invoices/:id", handlers.UpdateClientInvoice, scope("transactions:write"))
	api.DELETE("/client-invoices/:id", handlers.DeleteClientInvoice, scope("transactions:delete"))
}
