package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupEmployeePaymentRoutes(api *echo.Group) {
	api.GET("/employees/:id/payments", handlers.GetEmployeePayments, scopeOrSelf("employee-payments:read", selfEmployee), cache("employee-payments"))
	api.POST("/employee-payments", handlers.CreateEmployeePayment, criticalSync, scope("employee-payments:write"), cache("employee-payments", "balances", "analytics"))
	api.DELETE("/employee-payments/:id", handlers.DeleteBalanceTransaction, criticalSync, scope("employee-payments:delete"), cache("employee-payments", "balances", "analytics"))
	api.POST("/employee-adjustments", handlers.CreateEmployeeAdjustment, criticalSync, scope("employee-payments:write"), cache("employee-payments", "balances", "analytics"))
	api.POST("/employee-write-offs", handlers.CreateEmployeeWriteOff, criticalSync, scope("employee-payments:write"), cache("employee-payments", "balances", "analytics"))
}
