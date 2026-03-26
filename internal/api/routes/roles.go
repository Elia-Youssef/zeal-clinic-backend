package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupRoleRoutes(api *echo.Group) {
	api.GET("/roles", handlers.GetAllRoles)
	api.GET("/roles/:name", handlers.GetRoleByName)
	api.PUT("/roles/:name", handlers.UpdateRole, scope("roles:write"))
}
