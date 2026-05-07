package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupCurrencyRoutes(api *echo.Group) {
	api.GET("/currencies", handlers.GetAllCurrencies, scope("currencies:read"), cache("currencies"))
	api.GET("/currencies/dropdown", handlers.GetCurrencyDropdown, scope("currencies:read"), cache("currencies"))
	api.POST("/currencies", handlers.CreateCurrency, scope("currencies:write"), cache("currencies"))
	api.PUT("/currencies/:id", handlers.UpdateCurrency, scope("currencies:write"), cache("currencies"))
	api.DELETE("/currencies/:id", handlers.DeleteCurrency, scope("currencies:delete"), cache("currencies"))
}
