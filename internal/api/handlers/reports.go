package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"log"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

func GetReportForecast(c echo.Context) error {
	result, err := (&store.Report{}).GetForecast()
	if err != nil {
		log.Println("Error: GetReportForecast failed to generate forecast:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to generate forecast"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: result})
}

func GetReportProfitLoss(c echo.Context) error {
	from := c.QueryParam("from")
	to := c.QueryParam("to")

	if from == "" {
		// Default: first day of current month
		now := time.Now()
		from = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	}
	if to == "" {
		to = time.Now().Format("2006-01-02")
	}

	result, err := (&store.Report{}).GetProfitLoss(from, to)
	if err != nil {
		log.Println("Error: GetReportProfitLoss failed to generate P&L:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to generate P&L"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: result})
}
