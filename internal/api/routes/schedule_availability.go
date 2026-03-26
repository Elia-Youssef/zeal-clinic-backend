package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupScheduleAvailabilityRoutes(api *echo.Group) {
	api.GET("/schedule-availability", handlers.GetAllScheduleAvailability, scope("team:read"))
	api.POST("/schedule-availability", handlers.CreateScheduleAvailability, scope("team:write"))
	api.DELETE("/schedule-availability/:id", handlers.DeleteScheduleAvailability, scope("team:write"))
}
