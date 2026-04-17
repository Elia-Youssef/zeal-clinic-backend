package routes

import (
	"clinic-api/internal/api/handlers"
	"clinic-api/internal/api/middleware"

	"github.com/labstack/echo/v4"
)

func SetupBalanceAdjustmentRoutes(api *echo.Group) {
	api.POST("/balance-adjustments", handlers.CreateAdjustment, scope("transactions:write"), middleware.CacheMiddleware("balance-adjustments"))
	api.POST("/balance-write-offs", handlers.CreateWriteOff, scope("transactions:write"), middleware.CacheMiddleware("balance-adjustments"))
}
