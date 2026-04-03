package routes

import (
	"clinic-api/internal/api/handlers"
	"clinic-api/internal/api/middleware"

	"github.com/labstack/echo/v4"
)

func SetupRoleRoutes(api *echo.Group) {
	api.GET("/roles", handlers.GetAllRoles, middleware.CacheMiddleware("roles"))
	api.GET("/roles/dropdown", handlers.GetRoleDropdown, middleware.CacheMiddleware("roles"))
	api.GET("/roles/:name", handlers.GetRoleByName, middleware.CacheMiddleware("roles"))
	api.PUT("/roles/:name", handlers.UpdateRole, scope("roles:write"), middleware.CacheMiddleware("roles"))
}
