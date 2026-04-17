package routes

import (
	"clinic-api/internal/api/handlers"
	"clinic-api/internal/api/middleware"

	"github.com/labstack/echo/v4"
)

func SetupCountryRoutes(api *echo.Group) {
	api.GET("/countries", handlers.GetAllCountries, scope("patients:read"), middleware.CacheMiddleware("countries"))
	api.GET("/countries/dropdown", handlers.GetCountryDropdown, scope("patients:read"), middleware.CacheMiddleware("countries"))
}
