package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupNotificationRoutes(api *echo.Group) {
	api.GET("/notifications", handlers.GetAllNotifications, scope("bookings:read"))
	api.PUT("/notifications/:id/read", handlers.MarkNotificationRead, scope("bookings:read"))
	api.PUT("/notifications/read-all", handlers.MarkAllNotificationsRead, scope("bookings:read"))
}
