package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

// All analytics endpoints share the "analytics" cache key.
// Every domain whose writes change these numbers lists "analytics" in its own
// cache() call so non-GET requests bust the analytics cache alongside their own.
//
// The financial endpoints additionally require reports:read, so money figures
// stay behind the same gate as /reports.
func SetupAnalyticsRoutes(api *echo.Group) {
	// Section-grouped KPI tiles (each returns that section's values + deltas).
	// Each requires analytics:read AND the scope for the data it surfaces.
	api.GET("/analytics/money", handlers.GetAnalyticsMoney, scopeAll("analytics:read", "balances:read", "reports:read"), cache("analytics"))
	api.GET("/analytics/patients", handlers.GetAnalyticsPatients, scopeAll("analytics:read", "patients:read"), cache("analytics"))
	api.GET("/analytics/operations", handlers.GetAnalyticsOperations, scopeAll("analytics:read", "appointments:read"), cache("analytics"))
	api.GET("/analytics/inventory", handlers.GetAnalyticsInventory, scopeAll("analytics:read", "products:read"), cache("analytics"))

	// Breakdowns & ranked lists.
	api.GET("/analytics/demographics", handlers.GetAnalyticsDemographics, scopeAll("analytics:read", "patients:read"), cache("analytics"))
	api.GET("/analytics/referral-sources", handlers.GetAnalyticsReferralSources, scopeAll("analytics:read", "patients:read"), cache("analytics"))
	api.GET("/analytics/staff-performance", handlers.GetAnalyticsStaffPerformance, scopeAll("analytics:read", "employees:read"), cache("analytics"))
	api.GET("/analytics/procedures/top", handlers.GetAnalyticsTopProcedures, scopeAll("analytics:read", "procedures:read"), cache("analytics"))
	api.GET("/analytics/products/top", handlers.GetAnalyticsTopProducts, scopeAll("analytics:read", "products:read"), cache("analytics"))
	api.GET("/analytics/appointments/distribution", handlers.GetAnalyticsAppointmentDistribution, scopeAll("analytics:read", "appointments:read"), cache("analytics"))
	api.GET("/analytics/rooms/utilization", handlers.GetAnalyticsRoomUtilization, scopeAll("analytics:read", "rooms:read"), cache("analytics"))

	// Operational feeds.
	api.GET("/analytics/appointments/recent-today", handlers.GetAnalyticsRecentAppointmentsToday, scopeAll("analytics:read", "appointments:read"), cache("analytics"))
	api.GET("/analytics/transactions/recent", handlers.GetAnalyticsRecentTransactions, scopeAll("analytics:read", "balances:read", "reports:read"), cache("analytics"))

	// Time-series for graphs: metric/from/to/groupBy. The metric spans domains
	// (revenue, appointments, new-patients, …), and revenue is one of them, so
	// reports:read is required alongside analytics:read.
	api.GET("/analytics/series", handlers.GetAnalyticsSeries, scopeAll("analytics:read", "reports:read"), cache("analytics"))

	// Dashboard-style PDF export. It assembles every section, so it requires the
	// scopes for all of them.
	api.GET("/analytics/report/pdf", handlers.GetAnalyticsReportPDF, scopeAll("analytics:read", "balances:read", "reports:read", "patients:read", "appointments:read", "products:read"))
}
