package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupProductRoutes(api *echo.Group) {
	api.GET("/products", handlers.GetAllProducts, scope("inventory:read"), cache("products"))
	api.GET("/products/dropdown", handlers.GetProductDropdown, scope("inventory:read"), cache("products"))
	api.GET("/products/:id", handlers.GetProductByID, scope("inventory:read"), cache("products"))
	api.GET("/products/:id/invoices", handlers.GetProductInvoices, scope("transactions:read"))
	api.GET("/products/:id/prices", handlers.GetProductPrices, scope("inventory:read"))
	api.POST("/products", handlers.CreateProduct, scope("inventory:write"), cache("products", "analytics"))
	// Discount items reference product names; bust discounts on edit/delete.
	api.PUT("/products/:id", handlers.UpdateProduct, scope("inventory:write"), cache("products", "discounts", "analytics"))
	api.DELETE("/products/:id", handlers.DeleteProduct, scope("inventory:delete"), cache("products", "discounts", "analytics"))
}
