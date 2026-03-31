package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupProductCategoryRoutes(api *echo.Group) {
	api.GET("/product-categories", handlers.GetAllProductCategories, scope("inventory:read"))
	api.POST("/product-categories", handlers.CreateProductCategory, scope("inventory:write"))
	api.PUT("/product-categories/:id", handlers.UpdateProductCategory, scope("inventory:write"))
	api.DELETE("/product-categories/:id", handlers.DeleteProductCategory, scope("inventory:write"))
}
