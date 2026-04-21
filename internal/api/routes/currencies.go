package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupCurrencyRoutes(api *echo.Group) {
	api.GET("/currencies", handlers.GetAllCurrencies, scope("transactions:read"), cache("currencies"))
	api.GET("/currencies/dropdown", handlers.GetCurrencyDropdown, scope("transactions:read"), cache("currencies"))
	api.POST("/currencies", handlers.CreateCurrency, scope("transactions:write"), cache("currencies"))
	api.PUT("/currencies/:id", handlers.UpdateCurrency, scope("transactions:write"), cache("currencies"))
	api.DELETE("/currencies/:id", handlers.DeleteCurrency, scope("transactions:delete"), cache("currencies"))
}
