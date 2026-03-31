package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupBalanceRoutes(api *echo.Group) {
	api.GET("/balances", handlers.GetAllBalances, scope("transactions:read"))
	api.GET("/balances/:id", handlers.GetBalanceByID, scope("transactions:read"))
	api.POST("/balances", handlers.GetOrCreateBalance, scope("transactions:write"))
	api.GET("/balances/:id/transactions", handlers.GetBalanceTransactions, scope("transactions:read"))
}
