package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupBalanceRoutes(api *echo.Group) {
	api.GET("/balances/:type", handlers.GetAllBalances, scope("transactions:read"), cache("balances"))
}
