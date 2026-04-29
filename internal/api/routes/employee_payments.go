package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupEmployeePaymentRoutes(api *echo.Group) {
	api.GET("/employees/:id/payments", handlers.GetEmployeePayments, scope("transactions:read"), cache("employee-payments"))
	api.POST("/employee-payments", handlers.CreateEmployeePayment, scope("transactions:write"), cache("employee-payments", "balances", "analytics"))
	api.DELETE("/employee-payments/:id", handlers.DeleteBalanceTransaction, scope("transactions:delete"), cache("employee-payments", "balances", "analytics"))
	api.POST("/employee-adjustments", handlers.CreateEmployeeAdjustment, scope("transactions:write"), cache("employee-payments", "balances", "analytics"))
	api.POST("/employee-write-offs", handlers.CreateEmployeeWriteOff, scope("transactions:write"), cache("employee-payments", "balances", "analytics"))
}
