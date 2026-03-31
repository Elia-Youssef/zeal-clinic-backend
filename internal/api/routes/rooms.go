package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupRoomRoutes(api *echo.Group) {
	api.GET("/rooms", handlers.GetAllRooms, scope("rooms:read"))
	api.POST("/rooms", handlers.CreateRoom, scope("rooms:write"))
	api.PUT("/rooms/:id", handlers.UpdateRoom, scope("rooms:write"))
	api.DELETE("/rooms/:id", handlers.DeleteRoom, scope("rooms:delete"))
}
