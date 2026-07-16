package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupExpensePaymentRoutes(api *echo.Group) {
	api.GET("/expenses/:id/payments", handlers.GetExpensePayments, scope("payments:read"), cache("expense-payments"))
	api.POST("/expense-payments", handlers.CreateExpensePayment, criticalSync, scope("payments:write"), cache("expense-payments", "balances", "analytics", "reports"))
	api.DELETE("/expense-payments/:id", handlers.DeleteExpensePayment, criticalSync, scope("payments:delete"), cache("expense-payments", "balances", "analytics", "reports"))
	api.POST("/expense-adjustments", handlers.CreateExpenseAdjustment, criticalSync, scope("payments:write"), cache("expense-payments", "balances", "analytics", "reports"))
	api.POST("/expense-write-offs", handlers.CreateExpenseWriteOff, criticalSync, scope("payments:write"), cache("expense-payments", "balances", "analytics", "reports"))
}
