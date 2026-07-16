package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupProductRoutes(api *echo.Group) {
	api.GET("/products", handlers.GetAllProducts, scope("products:read"), cache("products"))
	api.GET("/products/dropdown", handlers.GetProductDropdown, scope("products:read"), cache("products"))
	api.GET("/products/:id", handlers.GetProductByID, scope("products:read"), cache("products"))
	api.GET("/products/:id/invoices", handlers.GetProductInvoices, scope("products:read"))
	api.GET("/products/:id/prices", handlers.GetProductPrices, scope("products:read"))
	api.POST("/products", handlers.CreateProduct, scope("products:write"), cache("products", "analytics", "reports"))
	api.PUT("/products/:id", handlers.UpdateProduct, scope("products:write"), cache("products", "discounts", "invoices", "analytics", "reports"))
	api.DELETE("/products/:id", handlers.DeleteProduct, scope("products:delete"), cache("products", "discounts", "invoices", "analytics", "reports"))
}
