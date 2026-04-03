package routes

import (
	"clinic-api/internal/api/handlers"
	"clinic-api/internal/api/middleware"

	"github.com/labstack/echo/v4"
)

func SetupProductRoutes(api *echo.Group) {
	api.GET("/products", handlers.GetAllProducts, scope("inventory:read"), middleware.CacheMiddleware("products"))
	api.GET("/products/dropdown", handlers.GetProductDropdown, scope("inventory:read"), middleware.CacheMiddleware("products"))
	api.GET("/products/:id", handlers.GetProductByID, scope("inventory:read"), middleware.CacheMiddleware("products"))
	api.POST("/products", handlers.CreateProduct, scope("inventory:write"), middleware.CacheMiddleware("products"))
	api.PUT("/products/:id", handlers.UpdateProduct, scope("inventory:write"), middleware.CacheMiddleware("products"))
	api.DELETE("/products/:id", handlers.DeleteProduct, scope("inventory:delete"), middleware.CacheMiddleware("products"))
}
