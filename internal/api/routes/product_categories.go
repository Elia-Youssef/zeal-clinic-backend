package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupProductCategoryRoutes(api *echo.Group) {
	api.GET("/product-categories", handlers.GetAllProductCategories, scope("product-categories:read"), cache("product-categories"))
	api.GET("/product-categories/dropdown", handlers.GetProductCategoryDropdown, scope("product-categories:read"), cache("product-categories"))
	api.POST("/product-categories", handlers.CreateProductCategory, scope("product-categories:write"), cache("product-categories"))
	api.PUT("/product-categories/:id", handlers.UpdateProductCategory, scope("product-categories:write"), cache("product-categories"))
	api.DELETE("/product-categories/:id", handlers.DeleteProductCategory, scope("product-categories:delete"), cache("product-categories"))
}
