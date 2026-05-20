package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupEmployeeRoutes(api *echo.Group) {
	api.GET("/employees", handlers.GetAllEmployees, scope("employees:read"), cache("employees"))
	api.GET("/employees/dropdown", handlers.GetEmployeeDropdown, scope("employees:read"), cache("employees"))
	api.GET("/employees/:id", handlers.GetEmployeeByID, scope("employees:read"), cache("employees"))
	// CreateEmployee can also create a linked user account.
	api.POST("/employees", handlers.CreateEmployee, scope("employees:write"), cache("employees", "users"))
	api.PUT("/employees/:id", handlers.UpdateEmployee, scope("employees:write"), cache("employees"))
	api.DELETE("/employees/:id", handlers.DeleteEmployee, scope("employees:delete"), cache("employees"))
}
