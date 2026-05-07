package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupExpensePaymentRoutes(api *echo.Group) {
	api.GET("/expenses/:id/payments", handlers.GetExpensePayments, scope("payments:read"), cache("expense-payments"))
	api.POST("/expense-payments", handlers.CreateExpensePayment, scope("payments:write"), cache("expense-payments", "balances", "analytics"))
	api.DELETE("/expense-payments/:id", handlers.DeleteBalanceTransaction, scope("payments:delete"), cache("expense-payments", "balances", "analytics"))
	api.POST("/expense-adjustments", handlers.CreateExpenseAdjustment, scope("payments:write"), cache("expense-payments", "balances", "analytics"))
	api.POST("/expense-write-offs", handlers.CreateExpenseWriteOff, scope("payments:write"), cache("expense-payments", "balances", "analytics"))
}
