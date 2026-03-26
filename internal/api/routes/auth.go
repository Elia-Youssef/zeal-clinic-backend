package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupAuthRoutes(api *echo.Group) {
	api.POST("/auth/login", handlers.Login)
	api.POST("/auth/logout", handlers.Logout)
}
