package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupEmployeeScheduleRoutes(api *echo.Group) {
	api.POST("/employee-schedules", handlers.CreateEmployeeSchedule, scope("employee-schedules:write"), cache("employee-schedules", "analytics"))
	api.PUT("/employee-schedules/:id", handlers.UpdateEmployeeSchedule, scope("employee-schedules:write"), cache("employee-schedules", "analytics"))
	api.DELETE("/employee-schedules/:id", handlers.DeleteEmployeeSchedule, scope("employee-schedules:delete"), cache("employee-schedules", "analytics"))
}
