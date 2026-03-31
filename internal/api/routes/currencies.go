package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupCurrencyRoutes(api *echo.Group) {
	api.GET("/currencies", handlers.GetAllCurrencies, scope("transactions:read"))
	api.POST("/currencies", handlers.CreateCurrency, scope("transactions:write"))
	api.PUT("/currencies/:id", handlers.UpdateCurrency, scope("transactions:write"))
	api.DELETE("/currencies/:id", handlers.DeleteCurrency, scope("transactions:delete"))
}
