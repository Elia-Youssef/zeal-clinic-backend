package routes

import (
	"clinic-api/internal/api/handlers"
	"clinic-api/internal/api/middleware"

	"github.com/labstack/echo/v4"
)

func SetupUserRoutes(api *echo.Group) {
	api.GET("/users", handlers.GetAllUsers, scope("team:read"), middleware.CacheMiddleware("users"))
	api.GET("/users/:id", handlers.GetUserByID, scope("team:read"), middleware.CacheMiddleware("users"))
	api.POST("/users", handlers.CreateUser, scope("team:write"), middleware.CacheMiddleware("users"))
	api.PUT("/users/:id", handlers.UpdateUser, scope("team:write"), middleware.CacheMiddleware("users"))
}
