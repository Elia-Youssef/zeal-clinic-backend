package routes

import (
	"clinic-api/internal/api/handlers"
	"clinic-api/internal/api/middleware"

	"github.com/labstack/echo/v4"
)

func SetupCurrencyRoutes(api *echo.Group) {
	api.GET("/currencies", handlers.GetAllCurrencies, scope("transactions:read"), middleware.CacheMiddleware("currencies"))
	api.GET("/currencies/dropdown", handlers.GetCurrencyDropdown, scope("transactions:read"), middleware.CacheMiddleware("currencies"))
	api.POST("/currencies", handlers.CreateCurrency, scope("transactions:write"), middleware.CacheMiddleware("currencies"))
	api.PUT("/currencies/:id", handlers.UpdateCurrency, scope("transactions:write"), middleware.CacheMiddleware("currencies"))
	api.DELETE("/currencies/:id", handlers.DeleteCurrency, scope("transactions:delete"), middleware.CacheMiddleware("currencies"))
}
