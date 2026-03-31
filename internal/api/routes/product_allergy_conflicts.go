package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupProductAllergyConflictRoutes(api *echo.Group) {
	api.GET("/products/:id/allergy-conflicts", handlers.GetProductAllergyConflicts, scope("inventory:read"))
	api.POST("/products/:id/allergy-conflicts", handlers.AddProductAllergyConflict, scope("inventory:write"))
	api.DELETE("/product-allergy-conflicts/:id", handlers.RemoveProductAllergyConflict, scope("inventory:write"))
}
