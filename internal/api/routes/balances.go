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

	// Transactions
	api.GET("/transactions", handlers.GetAllTransactions, scope("transactions:read"))
	api.POST("/transactions", handlers.CreateTransaction, scope("transactions:write"))

	// Invoices
	api.GET("/invoices", handlers.GetInvoices, scope("transactions:read"))
	api.GET("/invoices/:id", handlers.GetInvoiceByID, scope("transactions:read"))
	api.POST("/invoices", handlers.CreateInvoice, scope("transactions:write"))
	api.PUT("/invoices/:id", handlers.UpdateInvoice, scope("transactions:write"))
}
