package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupLebanonCityRoutes(api *echo.Group) {
	api.GET("/lebanon-cities", handlers.GetAllLebanonCities, cache("lebanon-cities"))
	api.GET("/lebanon-cities/dropdown", handlers.GetLebanonCityDropdown, cache("lebanon-cities"))
}
