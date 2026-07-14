package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupSupplierPaymentRoutes(api *echo.Group) {
	api.GET("/suppliers/:id/payments", handlers.GetSupplierPayments, scope("payments:read"), cache("supplier-payments"))
	api.POST("/supplier-payments", handlers.CreateSupplierPayment, criticalSync, scope("payments:write"), cache("supplier-payments", "balances", "analytics", "reports"))
	api.DELETE("/supplier-payments/:id", handlers.DeleteBalanceTransaction, criticalSync, scope("payments:delete"), cache("supplier-payments", "balances", "analytics", "reports"))
	api.POST("/supplier-adjustments", handlers.CreateSupplierAdjustment, criticalSync, scope("payments:write"), cache("supplier-payments", "balances", "analytics", "reports"))
	api.POST("/supplier-write-offs", handlers.CreateSupplierWriteOff, criticalSync, scope("payments:write"), cache("supplier-payments", "balances", "analytics", "reports"))
}
