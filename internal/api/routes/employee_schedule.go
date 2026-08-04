package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupEmployeeScheduleRoutes(api *echo.Group) {
	// Schedules are written a weekday at a time: the body carries every shift
	// worked on that weekday from startDate on. :id on the delete is any shift
	// of the version to drop.
	api.PUT("/employee-schedules/day", handlers.SaveEmployeeScheduleDay, scope("employee-schedules:write"), cache("employee-schedules", "analytics"))
	api.DELETE("/employee-schedules/:id", handlers.DeleteEmployeeSchedule, scope("employee-schedules:delete"), cache("employee-schedules", "analytics"))
}
