package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

// Write-only resource; reads come through GetEmployeeSchedule.

func CreateEmployeeSchedule(c echo.Context) error {
	var sa store.EmployeeSchedule
	if err := c.Bind(&sa); err != nil {
		log.Println("Error: CreateEmployeeSchedule invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	if err := sa.IsValid(); err != nil {
		log.Println("Error: CreateEmployeeSchedule validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
	}
	if err := sa.Create(); err != nil {
		log.Println("Error: CreateEmployeeSchedule failed to create employee schedule:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't create employee schedule"})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: sa})
}

func UpdateEmployeeSchedule(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdateEmployeeSchedule] invalid request body:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	sa := store.EmployeeSchedule{ID: c.Param("id")}
	if err := sa.Update(updates); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			log.Println("Error: [UpdateEmployeeSchedule] schedule entry not found:", c.Param("id"))
			return c.JSON(http.StatusNotFound, httpx.Response{Error: "Schedule entry not found"})
		}
		log.Println("Error: [UpdateEmployeeSchedule] failed to update schedule entry:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't update schedule entry"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: sa})
}

func DeleteEmployeeSchedule(c echo.Context) error {
	sa := store.EmployeeSchedule{ID: c.Param("id")}
	if err := sa.Delete(); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: [DeleteEmployeeSchedule] schedule entry not found:", err)
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "Schedule entry not found"})
	} else if err != nil {
		log.Println("Error: [DeleteEmployeeSchedule] failed to delete schedule entry:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't delete schedule entry"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
