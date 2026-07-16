package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupHRRoutes(api *echo.Group) {
	api.GET("/employees/:id/schedule", handlers.GetEmployeeSchedule, scopeOrSelf("hr:read", selfEmployee), cacheF("employee-schedules"))
	api.GET("/employees/:id/working-hours", handlers.GetEmployeeWorkingHours, scopeOrSelf("hr:read", selfEmployee), cache("employee-schedules"))

	// hr:read can create (handler forces status=pending + self employeeId for non-admins).
	api.POST("/employee-schedule-changes", handlers.CreateEmployeeScheduleChange, scope("hr:read"), cache("employee-schedules"))
	api.PUT("/employee-schedule-changes/:id", handlers.UpdateEmployeeScheduleChange, scope("hr:write"), cache("employee-schedules", "analytics"))
	api.POST("/employee-schedule-changes/:id/status", handlers.SetEmployeeScheduleChangeStatus, scope("hr:write"), cache("employee-schedules", "analytics"))
	api.DELETE("/employee-schedule-changes/:id", handlers.DeleteEmployeeScheduleChange, scope("hr:delete"), cache("employee-schedules", "analytics"))

	api.GET("/holidays", handlers.GetAllHolidays, scope("hr:read"), cache("holidays", "employee-schedules"))
	api.POST("/holidays", handlers.CreateHoliday, scope("hr:write"), cache("holidays", "employee-schedules", "appointments", "analytics"))
	api.PUT("/holidays/:id", handlers.UpdateHoliday, scope("hr:write"), cache("holidays", "employee-schedules", "appointments", "analytics"))
	api.DELETE("/holidays/:id", handlers.DeleteHoliday, scope("hr:delete"), cache("holidays", "employee-schedules", "appointments", "analytics"))
}
