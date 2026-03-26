package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupInventoryRoutes(api *echo.Group) {
	api.GET("/inventory", handlers.GetAllInventory, scope("inventory:read"))
	api.POST("/inventory", handlers.CreateInventoryItem, scope("inventory:write"))
	api.PUT("/inventory/:sku", handlers.UpdateInventoryItem, scope("inventory:write"))

	// Product categories
	api.GET("/product-categories", handlers.GetAllProductCategories, scope("inventory:read"))
	api.POST("/product-categories", handlers.CreateProductCategory, scope("inventory:write"))
	api.PUT("/product-categories/:id", handlers.UpdateProductCategory, scope("inventory:write"))
	api.DELETE("/product-categories/:id", handlers.DeleteProductCategory, scope("inventory:write"))

	// Product allergy conflicts
	api.GET("/inventory/:sku/allergy-conflicts", handlers.GetProductAllergyConflicts, scope("inventory:read"))
	api.POST("/inventory/:sku/allergy-conflicts", handlers.AddProductAllergyConflict, scope("inventory:write"))
	api.DELETE("/product-allergy-conflicts/:id", handlers.RemoveProductAllergyConflict, scope("inventory:write"))

	// Stock adjustments
	api.GET("/stock-adjustments", handlers.GetAllStockAdjustments, scope("inventory:read"))
	api.POST("/stock-adjustments", handlers.CreateStockAdjustment, scope("inventory:write"))
}
