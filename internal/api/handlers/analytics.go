package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"log"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
)

// Section-grouped KPI endpoints (each returns one section's tiles + deltas)

func GetAnalyticsMoney(c echo.Context) error {
	from, to := parseRange(c)
	data, err := (&store.Analytics{}).Money(from, to)
	if err != nil {
		log.Println("Error: GetAnalyticsMoney failed:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load financial analytics"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: data})
}

func GetAnalyticsPatients(c echo.Context) error {
	from, to := parseRange(c)
	data, err := (&store.Analytics{}).Patients(from, to)
	if err != nil {
		log.Println("Error: GetAnalyticsPatients failed:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load patient analytics"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: data})
}

func GetAnalyticsOperations(c echo.Context) error {
	from, to := parseRange(c)
	data, err := (&store.Analytics{}).Operations(from, to)
	if err != nil {
		log.Println("Error: GetAnalyticsOperations failed:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load operations analytics"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: data})
}

func GetAnalyticsInventory(c echo.Context) error {
	data, err := (&store.Analytics{}).Inventory()
	if err != nil {
		log.Println("Error: GetAnalyticsInventory failed:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load inventory analytics"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: data})
}

// List / breakdown endpoints

func GetAnalyticsDemographics(c echo.Context) error {
	data, err := (&store.Analytics{}).Demographics(parseLimit(c.QueryParam("cityLimit"), 5))
	if err != nil {
		log.Println("Error: GetAnalyticsDemographics failed:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load demographics"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: data})
}

func GetAnalyticsReferralSources(c echo.Context) error {
	from, to := parseRange(c)
	items, err := (&store.Analytics{}).ReferralSources(from, to)
	if err != nil {
		log.Println("Error: GetAnalyticsReferralSources failed:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load referral sources"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func GetAnalyticsStaffPerformance(c echo.Context) error {
	from, to := parseRange(c)
	items, err := (&store.Analytics{}).StaffPerformance(from, to)
	if err != nil {
		log.Println("Error: GetAnalyticsStaffPerformance failed:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load staff performance"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func GetAnalyticsTopProcedures(c echo.Context) error {
	from, to := parseRange(c)
	limit := parseLimit(c.QueryParam("limit"), 5)
	items, err := (&store.Analytics{}).TopProcedures(from, to, limit, c.QueryParam("by"))
	if err != nil {
		log.Println("Error: GetAnalyticsTopProcedures failed:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load top procedures"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func GetAnalyticsTopProducts(c echo.Context) error {
	from, to := parseRange(c)
	limit := parseLimit(c.QueryParam("limit"), 5)
	items, err := (&store.Analytics{}).TopProducts(from, to, limit)
	if err != nil {
		log.Println("Error: GetAnalyticsTopProducts failed:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load top products"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func GetAnalyticsAppointmentDistribution(c echo.Context) error {
	from, to := parseRange(c)
	data, err := (&store.Analytics{}).AppointmentDistribution(from, to)
	if err != nil {
		log.Println("Error: GetAnalyticsAppointmentDistribution failed:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load appointment distribution"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: data})
}

func GetAnalyticsRoomUtilization(c echo.Context) error {
	from, to := parseRange(c)
	items, err := (&store.Analytics{}).RoomUtilization(from, to)
	if err != nil {
		log.Println("Error: GetAnalyticsRoomUtilization failed:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load room utilization"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

// Operational feeds (kept, secondary)

func GetAnalyticsRecentAppointmentsToday(c echo.Context) error {
	limit := parseLimit(c.QueryParam("limit"), 5)
	items, err := (&store.Analytics{}).RecentAppointmentsToday(limit)
	if err != nil {
		log.Println("Error: GetAnalyticsRecentAppointmentsToday failed:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load recent appointments"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func GetAnalyticsRecentTransactions(c echo.Context) error {
	limit := parseLimit(c.QueryParam("limit"), 5)
	items, err := (&store.Analytics{}).RecentTransactions(limit)
	if err != nil {
		log.Println("Error: GetAnalyticsRecentTransactions failed:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load recent transactions"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

// Time-series

func GetAnalyticsSeries(c echo.Context) error {
	metric := c.QueryParam("metric")
	if metric == "" {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Metric is required"})
	}

	from, to := defaultDateRange(c.QueryParam("from"), c.QueryParam("to"), 29)

	groupBy := c.QueryParam("groupBy")
	if groupBy == "" {
		groupBy = "day"
	}

	points, err := (&store.Analytics{}).Series(store.SeriesParams{
		Metric:  metric,
		From:    from,
		To:      to,
		GroupBy: groupBy,
	})
	if err != nil {
		log.Println("Error: GetAnalyticsSeries failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Couldn't load analytics"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: points})
}

func parseLimit(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return def
	}
	return n
}
