package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupEmployeePaymentRoutes(api *echo.Group) {
	api.GET("/employees/:id/payments", handlers.GetEmployeePayments, scope("employee-payments:read"), cache("employee-payments"))
	api.POST("/employee-payments", handlers.CreateEmployeePayment, scope("employee-payments:write"), cache("employee-payments", "balances", "analytics"))
	api.DELETE("/employee-payments/:id", handlers.DeleteBalanceTransaction, scope("employee-payments:delete"), cache("employee-payments", "balances", "analytics"))
	api.POST("/employee-adjustments", handlers.CreateEmployeeAdjustment, scope("employee-payments:write"), cache("employee-payments", "balances", "analytics"))
	api.POST("/employee-write-offs", handlers.CreateEmployeeWriteOff, scope("employee-payments:write"), cache("employee-payments", "balances", "analytics"))
}
