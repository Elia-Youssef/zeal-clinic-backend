package routes

import (
	"clinic-api/internal/api/handlers"
	"clinic-api/internal/api/middleware"

	"github.com/labstack/echo/v4"
)

func SetupLebanonCityRoutes(api *echo.Group) {
	api.GET("/lebanon-cities", handlers.GetAllLebanonCities, scope("patients:read"), middleware.CacheMiddleware("lebanon-cities"))
	api.GET("/lebanon-cities/dropdown", handlers.GetLebanonCityDropdown, scope("patients:read"), middleware.CacheMiddleware("lebanon-cities"))
}
