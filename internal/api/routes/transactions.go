package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupTransactionRoutes(api *echo.Group) {
	api.GET("/transactions", handlers.GetAllTransactions, scope("transactions:read"))
	api.POST("/transactions", handlers.CreateTransaction, scope("transactions:write"))
}
