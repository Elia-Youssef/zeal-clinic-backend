package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupProductRoutes(api *echo.Group) {
	api.GET("/products", handlers.GetAllProducts, scope("inventory:read"))
	api.GET("/products/:id", handlers.GetProductByID, scope("inventory:read"))
	api.POST("/products", handlers.CreateProduct, scope("inventory:write"))
	api.PUT("/products/:id", handlers.UpdateProduct, scope("inventory:write"))
	api.DELETE("/products/:id", handlers.DeleteProduct, scope("inventory:delete"))
}
