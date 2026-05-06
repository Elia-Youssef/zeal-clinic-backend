package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

// schedule_availability is a write-only resource from the API surface; reads
// for the UI go through GetEmployeeSchedule.

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
	if err := sa.Create(); err != nil {
		log.Println("Error: CreateScheduleAvailability failed to create schedule availability:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to create schedule availability"})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: sa})
}

func UpdateScheduleAvailability(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdateScheduleAvailability] invalid request body:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	sa := store.ScheduleAvailability{ID: c.Param("id")}
	if err := sa.Update(updates); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			log.Println("Error: [UpdateScheduleAvailability] schedule entry not found:", c.Param("id"))
			return c.JSON(http.StatusNotFound, httpx.Response{Error: "schedule entry not found"})
		}
		log.Println("Error: [UpdateScheduleAvailability] failed to update schedule entry:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to update schedule entry"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: sa})
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
