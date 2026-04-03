package routes

import (
	"clinic-api/internal/api/handlers"
	"clinic-api/internal/api/middleware"

	"github.com/labstack/echo/v4"
)

func SetupRoomRoutes(api *echo.Group) {
	api.GET("/rooms", handlers.GetAllRooms, scope("rooms:read"), middleware.CacheMiddleware("rooms"))
	api.GET("/rooms/dropdown", handlers.GetRoomDropdown, scope("rooms:read"), middleware.CacheMiddleware("rooms"))
	api.POST("/rooms", handlers.CreateRoom, scope("rooms:write"), middleware.CacheMiddleware("rooms"))
	api.PUT("/rooms/:id", handlers.UpdateRoom, scope("rooms:write"), middleware.CacheMiddleware("rooms"))
	api.DELETE("/rooms/:id", handlers.DeleteRoom, scope("rooms:delete"), middleware.CacheMiddleware("rooms"))
}
