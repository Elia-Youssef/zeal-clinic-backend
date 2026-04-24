package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

// All analytics endpoints share the "analytics" cache key.
// Every domain whose writes change these numbers lists "analytics" in its own
// cache() call so non-GET requests bust the analytics cache alongside their own.
func SetupAnalyticsRoutes(api *echo.Group) {
	api.GET("/analytics/patients/total", handlers.GetAnalyticsTotalPatients, scope("reports:read"), cache("analytics"))
	api.GET("/analytics/patients/new-this-month", handlers.GetAnalyticsNewPatientsThisMonth, scope("reports:read"), cache("analytics"))

	api.GET("/analytics/appointments/counts", handlers.GetAnalyticsAppointmentCounts, scope("reports:read"), cache("analytics"))
	api.GET("/analytics/appointments/recent-today", handlers.GetAnalyticsRecentAppointmentsToday, scope("reports:read"), cache("analytics"))
	api.GET("/analytics/appointments/cancellation-rate", handlers.GetAnalyticsCancellationRate, scope("reports:read"), cache("analytics"))

	api.GET("/analytics/revenue/this-month", handlers.GetAnalyticsRevenueThisMonth, scope("reports:read"), cache("analytics"))
	api.GET("/analytics/revenue/outstanding", handlers.GetAnalyticsOutstandingReceivables, scope("reports:read"), cache("analytics"))

	api.GET("/analytics/transactions/recent", handlers.GetAnalyticsRecentTransactions, scope("reports:read"), cache("analytics"))

	api.GET("/analytics/procedures/completed-this-month", handlers.GetAnalyticsProceduresCompletedThisMonth, scope("reports:read"), cache("analytics"))
	api.GET("/analytics/procedures/top", handlers.GetAnalyticsTopProcedures, scope("reports:read"), cache("analytics"))

	api.GET("/analytics/inventory/low-stock", handlers.GetAnalyticsLowStock, scope("reports:read"), cache("analytics"))

	// Graph skeleton: metric/from/to/groupBy drive a time-series response.
	api.GET("/analytics/series", handlers.GetAnalyticsSeries, scope("reports:read"), cache("analytics"))
}
