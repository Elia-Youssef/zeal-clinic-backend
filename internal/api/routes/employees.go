package routes

import (
	"clinic-api/internal/api/handlers"
	"clinic-api/internal/api/middleware"

	"github.com/labstack/echo/v4"
)

func SetupEmployeeRoutes(api *echo.Group) {
	api.GET("/employees", handlers.GetAllEmployees, scope("team:read"), middleware.CacheMiddleware("employees"))
	api.GET("/employees/dropdown", handlers.GetEmployeeDropdown, scope("team:read"), middleware.CacheMiddleware("employees"))
	api.GET("/employees/:id", handlers.GetEmployeeByID, scope("team:read"), middleware.CacheMiddleware("employees"))
	api.POST("/employees", handlers.CreateEmployee, scope("team:write"), middleware.CacheMiddleware("employees"))
	api.PUT("/employees/:id", handlers.UpdateEmployee, scope("team:write"), middleware.CacheMiddleware("employees"))
	api.DELETE("/employees/:id", handlers.DeleteEmployee, scope("team:delete"), middleware.CacheMiddleware("employees"))
}
