package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupReportsRoutes(api *echo.Group) {
	api.GET("/reports/revenue", handlers.GetRevenueReport, scope("reports:read"), cache("reports"))
	api.GET("/reports/expenses", handlers.GetExpensesReport, scope("reports:read"), cache("reports"))
	api.GET("/reports/revenue/pdf", handlers.GetRevenueReportPDF, scope("reports:read"))
	api.GET("/reports/expenses/pdf", handlers.GetExpensesReportPDF, scope("reports:read"))
}
