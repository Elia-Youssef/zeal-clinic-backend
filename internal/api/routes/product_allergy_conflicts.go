package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupProductAllergyConflictRoutes(api *echo.Group) {
	api.GET("/products/:id/allergy-conflicts", handlers.GetProductAllergyConflicts, scope("product-allergy-conflicts:read"))
	api.POST("/products/:id/allergy-conflicts", handlers.AddProductAllergyConflict, scope("product-allergy-conflicts:write"))
	api.DELETE("/product-allergy-conflicts/:id", handlers.RemoveProductAllergyConflict, scope("product-allergy-conflicts:write"))
}
