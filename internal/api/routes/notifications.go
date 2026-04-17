package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupNotificationRoutes(api *echo.Group) {
	api.GET("/notifications", handlers.GetAllNotifications)
	api.GET("/notifications/unread-count", handlers.GetUnreadNotificationCount)
	api.POST("/notifications", handlers.CreateNotification)
	api.PUT("/notifications/:id/read", handlers.MarkNotificationRead)
	api.PUT("/notifications/read-all", handlers.MarkAllNotificationsRead)
	api.DELETE("/notifications/:id", handlers.DeleteNotification)
	api.DELETE("/notifications", handlers.DeleteAllNotifications)
}
