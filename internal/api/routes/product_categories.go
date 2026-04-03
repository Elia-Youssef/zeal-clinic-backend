package routes

import (
	"clinic-api/internal/api/handlers"
	"clinic-api/internal/api/middleware"

	"github.com/labstack/echo/v4"
)

func SetupProductCategoryRoutes(api *echo.Group) {
	api.GET("/product-categories", handlers.GetAllProductCategories, scope("inventory:read"), middleware.CacheMiddleware("product-categories"))
	api.GET("/product-categories/dropdown", handlers.GetProductCategoryDropdown, scope("inventory:read"), middleware.CacheMiddleware("product-categories"))
	api.POST("/product-categories", handlers.CreateProductCategory, scope("inventory:write"), middleware.CacheMiddleware("product-categories"))
	api.PUT("/product-categories/:id", handlers.UpdateProductCategory, scope("inventory:write"), middleware.CacheMiddleware("product-categories"))
	api.DELETE("/product-categories/:id", handlers.DeleteProductCategory, scope("inventory:write"), middleware.CacheMiddleware("product-categories"))
}
