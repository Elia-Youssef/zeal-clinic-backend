package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupEmployeeRoutes(api *echo.Group) {
	api.GET("/employees", handlers.GetAllEmployees, scope("team:read"))
	api.GET("/employees/:id", handlers.GetEmployeeByID, scope("team:read"))
	api.POST("/employees", handlers.CreateEmployee, scope("team:write"))
	api.PUT("/employees/:id", handlers.UpdateEmployee, scope("team:write"))
	api.DELETE("/employees/:id", handlers.DeleteEmployee, scope("team:delete"))
}
