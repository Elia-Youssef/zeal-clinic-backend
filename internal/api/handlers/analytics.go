package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
)

func GetAnalyticsTotalPatients(c echo.Context) error {
	n, err := (&store.Analytics{}).TotalPatients()
	if err != nil {
		log.Println("Error: GetAnalyticsTotalPatients failed:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch total patients"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: map[string]int{"total": n}})
}

func GetAnalyticsNewPatientsThisMonth(c echo.Context) error {
	n, err := (&store.Analytics{}).NewPatientsThisMonth()
	if err != nil {
		log.Println("Error: GetAnalyticsNewPatientsThisMonth failed:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch new patients"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: map[string]int{"total": n}})
}

func GetAnalyticsAppointmentCounts(c echo.Context) error {
	counts, err := (&store.Analytics{}).AppointmentCounts()
	if err != nil {
		log.Println("Error: GetAnalyticsAppointmentCounts failed:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch appointment counts"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: counts})
}

func GetAnalyticsRecentAppointmentsToday(c echo.Context) error {
	limit := parseLimit(c.QueryParam("limit"), 5)
	items, err := (&store.Analytics{}).RecentAppointmentsToday(limit)
	if err != nil {
		log.Println("Error: GetAnalyticsRecentAppointmentsToday failed:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch recent appointments"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func GetAnalyticsCancellationRate(c echo.Context) error {
	r, err := (&store.Analytics{}).CancellationRateThisMonth()
	if err != nil {
		log.Println("Error: GetAnalyticsCancellationRate failed:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch cancellation rate"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: r})
}

func GetAnalyticsRevenueThisMonth(c echo.Context) error {
	v, err := (&store.Analytics{}).RevenueThisMonth()
	if err != nil {
		log.Println("Error: GetAnalyticsRevenueThisMonth failed:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch revenue"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: map[string]float64{"total": v}})
}

func GetAnalyticsOutstandingReceivables(c echo.Context) error {
	v, err := (&store.Analytics{}).OutstandingReceivables()
	if err != nil {
		log.Println("Error: GetAnalyticsOutstandingReceivables failed:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch outstanding receivables"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: map[string]float64{"total": v}})
}

func GetAnalyticsRecentTransactions(c echo.Context) error {
	limit := parseLimit(c.QueryParam("limit"), 5)
	items, err := (&store.Analytics{}).RecentTransactions(limit)
	if err != nil {
		log.Println("Error: GetAnalyticsRecentTransactions failed:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch recent transactions"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func GetAnalyticsProceduresCompletedThisMonth(c echo.Context) error {
	n, err := (&store.Analytics{}).ProceduresCompletedThisMonth()
	if err != nil {
		log.Println("Error: GetAnalyticsProceduresCompletedThisMonth failed:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch procedures completed"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: map[string]int{"total": n}})
}

func GetAnalyticsTopProcedures(c echo.Context) error {
	limit := parseLimit(c.QueryParam("limit"), 5)
	items, err := (&store.Analytics{}).TopProceduresThisMonth(limit)
	if err != nil {
		log.Println("Error: GetAnalyticsTopProcedures failed:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch top procedures"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func GetAnalyticsLowStock(c echo.Context) error {
	n, err := (&store.Analytics{}).LowStockCount()
	if err != nil {
		log.Println("Error: GetAnalyticsLowStock failed:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch low-stock count"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: map[string]int{"total": n}})
}

func GetAnalyticsSeries(c echo.Context) error {
	metric := c.QueryParam("metric")
	if metric == "" {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "metric query parameter is required"})
	}

	from := c.QueryParam("from")
	to := c.QueryParam("to")
	if to == "" {
		to = time.Now().Format("2006-01-02")
	}
	if from == "" {
		from = time.Now().AddDate(0, 0, -29).Format("2006-01-02")
	}

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
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: err.Error()})
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
