package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupCountryRoutes(api *echo.Group) {
	api.GET("/countries", handlers.GetAllCountries, cache("countries"))
	api.GET("/countries/dropdown", handlers.GetCountryDropdown, cache("countries"))
}
