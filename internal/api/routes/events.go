package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupEventRoutes(api *echo.Group) {
	api.GET("/events", handlers.StreamEvents)
}
