package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

// SetupDebugRoutes registers the routes that exist only in development runs
// and in the automated tests; an ordinary built server never registers them.
func SetupDebugRoutes(api *echo.Group) {
	api.POST("/notifications/test", handlers.SendTestNotification)
}
