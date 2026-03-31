package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllScheduleAvailability(c echo.Context) error {
	employeeID := c.QueryParam("employeeId")
	sa := &models.ScheduleAvailability{}
	var items []models.ScheduleAvailability
	var err error
	if employeeID != "" {
		items, err = sa.GetByEmployee(employeeID)
	} else {
		items, err = sa.GetAll()
	}
	if err != nil {
		log.Println("Error: GetAllScheduleAvailability failed to fetch schedule availability:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch schedule availability"})
	}
	if items == nil {
		items = []models.ScheduleAvailability{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func CreateScheduleAvailability(c echo.Context) error {
	var sa models.ScheduleAvailability
	if err := c.Bind(&sa); err != nil {
		log.Println("Error: CreateScheduleAvailability invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	if err := sa.IsValid(); err != nil {
		log.Println("Error: CreateScheduleAvailability validation failed:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed", Data: err})
	}
	sa.CreatedAt = models.DateNow()
	if err := sa.Create(); err != nil {
		log.Println("Error: CreateScheduleAvailability failed to create schedule availability:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create schedule availability"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: sa})
}

func DeleteScheduleAvailability(c echo.Context) error {
	sa := models.ScheduleAvailability{ID: c.Param("id")}
	if err := sa.Delete(); err == sql.ErrNoRows {
		log.Println("Error: [DeleteScheduleAvailability] schedule entry not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "schedule entry not found"})
	} else if err != nil {
		log.Println("Error: [DeleteScheduleAvailability] failed to delete schedule entry:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete schedule entry"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}
