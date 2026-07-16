package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupServerInfoRoutes(api *echo.Group) {
	api.GET("/server-info", handlers.GetServerInfo, cache("server-info"))
}
