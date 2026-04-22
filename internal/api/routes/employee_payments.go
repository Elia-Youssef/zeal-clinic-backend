package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupEmployeePaymentRoutes(api *echo.Group) {
	api.GET("/employees/:id/payments", handlers.GetEmployeePayments, scope("transactions:read"), cache("employee-payments"))
	api.POST("/employee-payments", handlers.CreateEmployeePayment, scope("transactions:write"), cache("employee-payments", "balances"))
}
