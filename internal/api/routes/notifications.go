package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupNotificationRoutes(api *echo.Group) {
	api.GET("/notifications", handlers.GetAllNotifications)
	api.GET("/notifications/unread-count", handlers.GetUnreadNotificationCount)
	api.PUT("/notifications/:id/read", handlers.MarkNotificationRead)
	api.PUT("/notifications/read-all", handlers.MarkAllNotificationsRead)
	api.DELETE("/notifications/:id", handlers.DeleteNotification)
}

// SetupDebugRoutes registers the routes that exist only in development runs
// and in the automated tests; a built server never registers them.
func SetupDebugRoutes(api *echo.Group) {
	api.POST("/notifications/test", handlers.SendTestNotification)
}
