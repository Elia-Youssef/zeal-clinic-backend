package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupProductRoutes(api *echo.Group) {
	api.GET("/products", handlers.GetAllProducts, scope("inventory:read"), cache("products"))
	api.GET("/products/dropdown", handlers.GetProductDropdown, scope("inventory:read"), cache("products"))
	api.GET("/products/:id", handlers.GetProductByID, scope("inventory:read"), cache("products"))
	api.POST("/products", handlers.CreateProduct, scope("inventory:write"), cache("products"))
	api.PUT("/products/:id", handlers.UpdateProduct, scope("inventory:write"), cache("products"))
	api.DELETE("/products/:id", handlers.DeleteProduct, scope("inventory:delete"), cache("products"))
}
