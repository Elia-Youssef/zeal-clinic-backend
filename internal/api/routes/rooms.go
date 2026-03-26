package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupRoomRoutes(api *echo.Group) {
	api.GET("/rooms", handlers.GetAllRooms, scope("rooms:read"))
}
