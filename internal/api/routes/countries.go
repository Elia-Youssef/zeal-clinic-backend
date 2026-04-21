package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupCountryRoutes(api *echo.Group) {
	api.GET("/countries", handlers.GetAllCountries, scope("patients:read"), cache("countries"))
	api.GET("/countries/dropdown", handlers.GetCountryDropdown, scope("patients:read"), cache("countries"))
}
