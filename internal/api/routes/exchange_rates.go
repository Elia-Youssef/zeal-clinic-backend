package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupExchangeRateRoutes(api *echo.Group) {
	api.GET("/exchange-rates", handlers.GetAllExchangeRates, scope("transactions:read"))
	api.GET("/exchange-rates/latest", handlers.GetLatestExchangeRate, scope("transactions:read"))
	api.POST("/exchange-rates", handlers.CreateExchangeRate, scope("transactions:write"))
}
