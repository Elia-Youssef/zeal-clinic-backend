package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupAuthRoutes(public *echo.Group, protected *echo.Group) {
	public.POST("/auth/login", handlers.Login)
	public.POST("/auth/logout", handlers.Logout)
	protected.GET("/auth/verify", handlers.Verify)
}
