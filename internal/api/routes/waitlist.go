package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupWaitlistRoutes(api *echo.Group) {
	api.GET("/waitlist", handlers.GetAllWaitlist, scope("appointments:read"))
	api.POST("/waitlist", handlers.CreateWaitlistEntry, scope("appointments:write"))
	api.PUT("/waitlist/:id", handlers.UpdateWaitlistStatus, scope("appointments:write"))
	api.DELETE("/waitlist/:id", handlers.DeleteWaitlistEntry, scope("appointments:delete"))
}
