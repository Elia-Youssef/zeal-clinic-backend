package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupAllergyRoutes(api *echo.Group) {
	api.GET("/allergies", handlers.GetAllAllergies, scope("patients:read"))
	api.POST("/allergies", handlers.CreateAllergy, scope("patients:write"))
	api.PUT("/allergies/:id", handlers.UpdateAllergy, scope("patients:write"))
	api.DELETE("/allergies/:id", handlers.DeleteAllergy, scope("patients:delete"))
}
