package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupLebanonCityRoutes(api *echo.Group) {
	api.GET("/lebanon-cities", handlers.GetAllLebanonCities, scope("patients:read"), cache("lebanon-cities"))
	api.GET("/lebanon-cities/dropdown", handlers.GetLebanonCityDropdown, scope("patients:read"), cache("lebanon-cities"))
}
