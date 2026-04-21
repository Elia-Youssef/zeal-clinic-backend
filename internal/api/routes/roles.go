package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupRoleRoutes(api *echo.Group) {
	api.GET("/roles", handlers.GetAllRoles, cache("roles"))
	api.GET("/roles/dropdown", handlers.GetRoleDropdown, cache("roles"))
	api.GET("/roles/:name", handlers.GetRoleByName, cache("roles"))
	api.PUT("/roles/:name", handlers.UpdateRole, scope("roles:write"), cache("roles"))
}
