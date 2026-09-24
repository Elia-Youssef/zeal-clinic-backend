package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupEmployeeSalaryRoutes(api *echo.Group) {
	api.GET("/employees/:id/salaries", handlers.GetEmployeeSalaries, scopeOrSelf("employee-salaries:read", selfEmployee))
	api.POST("/employees/:id/salaries", handlers.CreateEmployeeSalary, scope("employee-salaries:write"), cache("employees"))
	api.PUT("/employee-salaries/:id", handlers.UpdateEmployeeSalary, scope("employee-salaries:write"), cache("employees"))
	api.DELETE("/employee-salaries/:id", handlers.DeleteEmployeeSalary, scope("employee-salaries:delete"), cache("employees"))
	api.POST("/employee-salaries/prepare", handlers.PrepareEmployeeSalaries, criticalSync, scope("employee-salaries:write"), cache("employee-payments", "balances", "analytics"))
	api.GET("/employees/:id/prepared-salaries", handlers.GetEmployeePreparedSalaries, scopeOrSelf("employee-salaries:read", selfEmployee))
	api.PATCH("/employee-salary-preparations/:id", handlers.UpdateEmployeeSalaryPreparation, criticalSync, scope("employee-salaries:write"), cache("employee-payments", "balances", "analytics"))
	api.DELETE("/employee-salary-preparations/:id", handlers.DeleteEmployeeSalaryPreparation, criticalSync, scope("employee-salaries:delete"), cache("employee-payments", "balances", "analytics"))
}
