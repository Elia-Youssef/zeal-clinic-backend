package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupSearchRoutes(api *echo.Group) {
	api.GET("/search", handlers.SearchAll)
}
