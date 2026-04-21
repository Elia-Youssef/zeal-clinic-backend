package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupUserRoutes(api *echo.Group) {
	api.GET("/users", handlers.GetAllUsers, scope("team:read"), cache("users"))
	api.GET("/users/:id", handlers.GetUserByID, scope("team:read"), cache("users"))
	api.POST("/users", handlers.CreateUser, scope("team:write"), cache("users"))
	api.PUT("/users/:id", handlers.UpdateUser, scope("team:write"), cache("users"))
}
