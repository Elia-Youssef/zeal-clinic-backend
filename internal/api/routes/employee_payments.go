package routes

import (
	"clinic-api/internal/api/handlers"
	"clinic-api/internal/api/middleware"

	"github.com/labstack/echo/v4"
)

func SetupEmployeePaymentRoutes(api *echo.Group) {
	api.GET("/employees/:id/payments", handlers.GetEmployeePayments, scope("transactions:read"), middleware.CacheMiddleware("employee-payments"))
	api.POST("/employee-payments", handlers.CreateEmployeePayment, scope("transactions:write"), middleware.CacheMiddleware("employee-payments"))
}
