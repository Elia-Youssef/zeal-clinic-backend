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
	api.POST("/employee-salaries/prepare", handlers.PrepareEmployeeSalaries, scope("transactions:write"), cache("employee-payments", "balances", "analytics"))
	api.GET("/employees/:id/prepared-salaries", handlers.GetEmployeePreparedSalaries, scope("team:read"))
	api.PATCH("/employee-salary-preparations/:id", handlers.UpdateEmployeeSalaryPreparation, scope("transactions:write"), cache("employee-payments", "balances", "analytics"))
	api.DELETE("/employee-salary-preparations/:id", handlers.DeleteEmployeeSalaryPreparation, scope("transactions:delete"), cache("employee-payments", "balances", "analytics"))
}
