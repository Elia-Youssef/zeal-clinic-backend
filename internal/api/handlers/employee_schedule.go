package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

// Write-only resource; reads come through GetEmployeeSchedule.

// SaveEmployeeScheduleDay writes the complete shift set for one weekday. The
// shifts in the body are the shifts that exist from startDate on; a weekday may
// carry several as long as they don't overlap, and an empty list turns it into a
// day off. Saving the whole day at once keeps a multi-shift weekday from ever
// being half-written.
func SaveEmployeeScheduleDay(c echo.Context) error {
	var version store.EmployeeScheduleVersion
	if err := c.Bind(&version); err != nil {
		log.Println("Error: [SaveEmployeeScheduleDay] invalid request body:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	shifts, err := version.Save()
	if err != nil {
		return storeError(c, err, "Schedule entry not found", "Couldn't save employee schedule")
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: shifts})
}

// DeleteEmployeeSchedule removes the whole version the given shift belongs to,
// leaving the dates it covered uncovered.
func DeleteEmployeeSchedule(c echo.Context) error {
	sa := store.EmployeeSchedule{ID: c.Param("id")}
	if err := sa.Delete(); err != nil {
		return storeError(c, err, "Schedule entry not found", "Couldn't delete schedule entry")
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
