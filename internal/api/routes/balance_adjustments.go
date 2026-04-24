package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupBalanceAdjustmentRoutes(api *echo.Group) {
	// Adjustments and write-offs can touch any balance pair; invalidate every
	// payment-list cache so those views reflect the new transaction.
	api.POST("/balance-adjustments", handlers.CreateAdjustment, scope("transactions:write"),
		cache("balances", "client-payments", "supplier-payments", "employee-payments", "analytics"))
	api.POST("/balance-write-offs", handlers.CreateWriteOff, scope("transactions:write"),
		cache("balances", "client-payments", "supplier-payments", "employee-payments", "analytics"))
}
