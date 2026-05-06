package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupHRRoutes(api *echo.Group) {
	// Single unified weekly read: returns days, templates, vacations, holidays,
	// and monthly hours for the week containing ?date= for one employee.
	api.GET("/employees/:id/schedule", handlers.GetEmployeeSchedule, scope("schedule:read"), cache("employee-schedules"))

	// Any authenticated user with schedule:read can request a vacation; the
	// handler forces status=pending and self-employeeId for non-admin users.
	api.POST("/employee-vacations", handlers.CreateEmployeeVacation, scope("schedule:read"), cache("employee-schedules"))
	api.PUT("/employee-vacations/:id", handlers.UpdateEmployeeVacation, scope("schedule:write"), cache("employee-schedules", "analytics"))
	api.POST("/employee-vacations/:id/status", handlers.SetEmployeeVacationStatus, scope("schedule:write"), cache("employee-schedules", "analytics"))
	api.DELETE("/employee-vacations/:id", handlers.DeleteEmployeeVacation, scope("schedule:delete"), cache("employee-schedules", "analytics"))

	api.GET("/holidays", handlers.GetAllHolidays, scope("schedule:read"), cache("holidays", "employee-schedules"))
	api.POST("/holidays", handlers.CreateHoliday, scope("schedule:write"), cache("holidays", "employee-schedules", "analytics"))
	api.PUT("/holidays/:id", handlers.UpdateHoliday, scope("schedule:write"), cache("holidays", "employee-schedules", "analytics"))
	api.DELETE("/holidays/:id", handlers.DeleteHoliday, scope("schedule:delete"), cache("holidays", "employee-schedules", "analytics"))
}
