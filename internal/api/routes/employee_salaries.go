package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupEmployeeSalaryRoutes(api *echo.Group) {
	api.GET("/employees/:id/salaries", handlers.GetEmployeeSalaries, scope("employee-salaries:read"))
	api.POST("/employees/:id/salaries", handlers.CreateEmployeeSalary, scope("employee-salaries:write"))
	api.PUT("/employee-salaries/:id", handlers.UpdateEmployeeSalary, scope("employee-salaries:write"))
	api.DELETE("/employee-salaries/:id", handlers.DeleteEmployeeSalary, scope("employee-salaries:delete"))
	api.POST("/employee-salaries/prepare", handlers.PrepareEmployeeSalaries, scope("employee-salaries:write"), cache("employee-payments", "balances", "analytics"))
	api.GET("/employees/:id/prepared-salaries", handlers.GetEmployeePreparedSalaries, scope("employee-salaries:read"))
	api.PATCH("/employee-salary-preparations/:id", handlers.UpdateEmployeeSalaryPreparation, scope("employee-salaries:write"), cache("employee-payments", "balances", "analytics"))
	api.DELETE("/employee-salary-preparations/:id", handlers.DeleteEmployeeSalaryPreparation, scope("employee-salaries:delete"), cache("employee-payments", "balances", "analytics"))
}
