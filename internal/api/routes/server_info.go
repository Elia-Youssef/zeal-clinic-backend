package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupServerInfoRoutes(pub *echo.Group) {
	pub.GET("/server-url", handlers.GetServerURL)
}
