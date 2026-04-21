package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupBalanceAdjustmentRoutes(api *echo.Group) {
	api.POST("/balance-adjustments", handlers.CreateAdjustment, scope("transactions:write"), cache("balance-adjustments"))
	api.POST("/balance-write-offs", handlers.CreateWriteOff, scope("transactions:write"), cache("balance-adjustments"))
}
