package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupEmployeeSalaryRoutes(api *echo.Group) {
	api.GET("/employees/:id/salaries", handlers.GetEmployeeSalaries, scope("team:read"))
	api.POST("/employees/:id/salaries", handlers.CreateEmployeeSalary, scope("team:write"))
	api.PUT("/employee-salaries/:id", handlers.UpdateEmployeeSalary, scope("team:write"))
	api.DELETE("/employee-salaries/:id", handlers.DeleteEmployeeSalary, scope("team:delete"))
}
