package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupScheduleAvailabilityRoutes(api *echo.Group) {
	api.POST("/schedule-availability", handlers.CreateScheduleAvailability, scope("schedule-availability:write"), cache("employee-schedules", "analytics"))
	api.PUT("/schedule-availability/:id", handlers.UpdateScheduleAvailability, scope("schedule-availability:write"), cache("employee-schedules", "analytics"))
	api.DELETE("/schedule-availability/:id", handlers.DeleteScheduleAvailability, scope("schedule-availability:delete"), cache("employee-schedules", "analytics"))
}
