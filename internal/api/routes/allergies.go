package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupAllergyRoutes(api *echo.Group) {
	api.GET("/allergies", handlers.GetAllAllergies, scope("allergies:read"), cache("allergies"))
	api.GET("/allergies/dropdown", handlers.GetAllergyDropdown, scope("allergies:read"), cache("allergies"))
	api.POST("/allergies", handlers.CreateAllergy, scope("allergies:write"), cache("allergies"))
	api.PUT("/allergies/:id", handlers.UpdateAllergy, scope("allergies:write"), cache("allergies"))
	api.DELETE("/allergies/:id", handlers.DeleteAllergy, scope("allergies:delete"), cache("allergies"))
}
