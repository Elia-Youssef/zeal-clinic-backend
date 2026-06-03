package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupUserRoutes(api *echo.Group) {
	api.GET("/users", handlers.GetAllUsers, scope("users:read"), cache("users"))
	api.GET("/users/:id", handlers.GetUserByID, scopeOrSelf("users:read", selfUser), cache("users"))
	api.GET("/users/:id/actions", handlers.GetUserActions, scope("users:read"))
	api.POST("/users", handlers.CreateUser, scope("users:write"), cache("users"))
	api.PUT("/users/:id", handlers.UpdateUser, scope("users:write"), cache("users"))
}
