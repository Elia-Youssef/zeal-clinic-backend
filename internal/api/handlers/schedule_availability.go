package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllScheduleAvailability(c echo.Context) error {
	employeeID := c.QueryParam("employeeId")
	sa := &store.ScheduleAvailability{}
	var items []store.ScheduleAvailability
	var total int
	var err error
	if employeeID != "" {
		items, err = sa.GetByEmployee(employeeID)
		total = len(items)
	} else {
		params := parseListParams(c)
		items, total, err = sa.GetAll(params)
	}
	if err != nil {
		log.Println("Error: GetAllScheduleAvailability failed to fetch schedule availability:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch schedule availability"})
	}
	if items == nil {
		items = []store.ScheduleAvailability{}
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: items, Total: total}})
}

func CreateScheduleAvailability(c echo.Context) error {
	var sa store.ScheduleAvailability
	if err := c.Bind(&sa); err != nil {
		log.Println("Error: CreateScheduleAvailability invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	if err := sa.IsValid(); err != nil {
		log.Println("Error: CreateScheduleAvailability validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "validation failed"})
	}
	sa.CreatedAt = store.DateNow()
	if err := sa.Create(); err != nil {
		log.Println("Error: CreateScheduleAvailability failed to create schedule availability:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to create schedule availability"})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: sa})
}

func DeleteScheduleAvailability(c echo.Context) error {
	sa := store.ScheduleAvailability{ID: c.Param("id")}
	if err := sa.Delete(); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: [DeleteScheduleAvailability] schedule entry not found:", err)
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "schedule entry not found"})
	} else if err != nil {
		log.Println("Error: [DeleteScheduleAvailability] failed to delete schedule entry:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to delete schedule entry"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
