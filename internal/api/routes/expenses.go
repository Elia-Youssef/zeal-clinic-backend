package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupExpenseRoutes(api *echo.Group) {
	api.GET("/expenses", handlers.GetAllExpenses, scope("transactions:read"), cache("expenses"))
	api.GET("/expenses/dropdown", handlers.GetExpenseDropdown, scope("transactions:read"), cache("expenses"))
	api.GET("/expenses/:id", handlers.GetExpenseByID, scope("transactions:read"), cache("expenses"))
	api.POST("/expenses", handlers.CreateExpense, scope("transactions:write"), cache("expenses"))
	api.PUT("/expenses/:id", handlers.UpdateExpense, scope("transactions:write"), cache("expenses"))
	api.DELETE("/expenses/:id", handlers.DeleteExpense, scope("transactions:delete"), cache("expenses"))
}
