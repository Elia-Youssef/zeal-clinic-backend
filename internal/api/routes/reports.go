package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupReportRoutes(api *echo.Group) {
	api.GET("/reports/forecast", handlers.GetReportForecast, scope("reports:read"))
	api.GET("/reports/profit-loss", handlers.GetReportProfitLoss, scope("reports:read"))
}
