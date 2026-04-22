package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupRoomRoutes(api *echo.Group) {
	api.GET("/rooms", handlers.GetAllRooms, scope("rooms:read"), cache("rooms"))
	api.GET("/rooms/dropdown", handlers.GetRoomDropdown, scope("rooms:read"), cache("rooms"))
	// Appointment views reference room info; bust them when rooms change.
	api.POST("/rooms", handlers.CreateRoom, scope("rooms:write"), cache("rooms", "appointments"))
	api.PUT("/rooms/:id", handlers.UpdateRoom, scope("rooms:write"), cache("rooms", "appointments"))
	api.DELETE("/rooms/:id", handlers.DeleteRoom, scope("rooms:delete"), cache("rooms", "appointments"))
}
