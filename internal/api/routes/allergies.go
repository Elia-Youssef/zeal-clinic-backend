package routes

import (
	"clinic-api/internal/api/handlers"
	"clinic-api/internal/api/middleware"

	"github.com/labstack/echo/v4"
)

func SetupAllergyRoutes(api *echo.Group) {
	api.GET("/allergies", handlers.GetAllAllergies, scope("patients:read"), middleware.CacheMiddleware("allergies"))
	api.GET("/allergies/dropdown", handlers.GetAllergyDropdown, scope("patients:read"), middleware.CacheMiddleware("allergies"))
	api.POST("/allergies", handlers.CreateAllergy, scope("patients:write"), middleware.CacheMiddleware("allergies"))
	api.PUT("/allergies/:id", handlers.UpdateAllergy, scope("patients:write"), middleware.CacheMiddleware("allergies"))
	api.DELETE("/allergies/:id", handlers.DeleteAllergy, scope("patients:delete"), middleware.CacheMiddleware("allergies"))
}
