package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupHRRoutes(api *echo.Group) {
	// Single unified weekly read: returns days, templates, vacations, holidays,
	// and monthly hours for the week containing ?date= for one employee.
	api.GET("/employees/:id/schedule", handlers.GetEmployeeSchedule, scope("hr:read"), cacheF("employee-schedules"))

	// Any authenticated user with hr:read can request a vacation; the handler
	// forces status=pending and self-employeeId for non-admin users.
	api.POST("/employee-vacations", handlers.CreateEmployeeVacation, scope("hr:read"), cache("employee-schedules"))
	api.PUT("/employee-vacations/:id", handlers.UpdateEmployeeVacation, scope("hr:write"), cache("employee-schedules", "analytics"))
	api.POST("/employee-vacations/:id/status", handlers.SetEmployeeVacationStatus, scope("hr:write"), cache("employee-schedules", "analytics"))
	api.DELETE("/employee-vacations/:id", handlers.DeleteEmployeeVacation, scope("hr:delete"), cache("employee-schedules", "analytics"))

	api.GET("/holidays", handlers.GetAllHolidays, scope("hr:read"), cache("holidays", "employee-schedules"))
	api.POST("/holidays", handlers.CreateHoliday, scope("hr:write"), cache("holidays", "employee-schedules", "analytics"))
	api.PUT("/holidays/:id", handlers.UpdateHoliday, scope("hr:write"), cache("holidays", "employee-schedules", "analytics"))
	api.DELETE("/holidays/:id", handlers.DeleteHoliday, scope("hr:delete"), cache("holidays", "employee-schedules", "analytics"))
}
