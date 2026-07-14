package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupClientPaymentRoutes(api *echo.Group) {
	api.GET("/patients/:id/payments", handlers.GetClientPayments, scope("payments:read"), cache("client-payments"))
	api.POST("/client-payments", handlers.CreateClientPayment, criticalSync, scope("payments:write"), cache("client-payments", "balances", "analytics"))
	api.DELETE("/client-payments/:id", handlers.DeleteBalanceTransaction, criticalSync, scope("payments:delete"), cache("client-payments", "balances", "analytics"))
	api.POST("/client-refunds", handlers.CreateClientRefund, criticalSync, scope("payments:write"), cache("client-payments", "balances", "analytics"))
	api.POST("/client-adjustments", handlers.CreateClientAdjustment, criticalSync, scope("payments:write"), cache("client-payments", "balances", "analytics"))
	api.POST("/client-write-offs", handlers.CreateClientWriteOff, criticalSync, scope("payments:write"), cache("client-payments", "balances", "analytics"))
}
