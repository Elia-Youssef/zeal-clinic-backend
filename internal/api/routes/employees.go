package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupEmployeeRoutes(api *echo.Group) {
	api.GET("/employees", handlers.GetAllEmployees, scope("team:read"), cache("employees"))
	api.GET("/employees/dropdown", handlers.GetEmployeeDropdown, scope("team:read"), cache("employees"))
	api.GET("/employees/:id", handlers.GetEmployeeByID, scope("team:read"), cache("employees"))
	api.POST("/employees", handlers.CreateEmployee, scope("team:write"), cache("employees"))
	api.PUT("/employees/:id", handlers.UpdateEmployee, scope("team:write"), cache("employees"))
	api.DELETE("/employees/:id", handlers.DeleteEmployee, scope("team:delete"), cache("employees"))
}
