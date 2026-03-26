package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllWaitlist(c echo.Context) error {
	status := c.QueryParam("status")
	list, err := (&models.WaitlistEntry{}).GetAll(status)
	if err != nil {
		log.Println("Error: GetAllWaitlist failed to fetch waitlist")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch waitlist"})
	}
	if list == nil {
		list = []models.WaitlistEntry{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: list})
}

func CreateWaitlistEntry(c echo.Context) error {
	var e models.WaitlistEntry
	if err := c.Bind(&e); err != nil {
		log.Println("Error: CreateWaitlistEntry invalid request")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}

	v := NewValidator()
	v.Required("patientId", e.PatientID, "Patient ID")
	v.Required("procedureType", e.ProcedureType, "Procedure type")
	v.OneOf("urgency", e.Urgency, []string{"low", "normal", "high", ""}, "Urgency")
	v.Date("preferredFrom", e.PreferredFrom)
	v.Date("preferredTo", e.PreferredTo)
	if v.HasErrors() {
		log.Println("Error: CreateWaitlistEntry validation failed")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed", Data: v.Fields})
	}

	if err := e.Create(); err != nil {
		log.Println("Error: CreateWaitlistEntry failed to add to waitlist")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to add to waitlist"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: e})
}

func UpdateWaitlistStatus(c echo.Context) error {
	var body struct {
		Status        string `json:"status"`
		AppointmentID string `json:"appointmentId"`
	}
	if err := c.Bind(&body); err != nil {
		log.Println("Error: UpdateWaitlistStatus invalid request")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}

	v := NewValidator()
	v.Required("status", body.Status, "Status")
	v.OneOf("status", body.Status, []string{"waiting", "contacted", "booked", "declined", "cancelled"}, "Status")
	if v.HasErrors() {
		log.Println("Error: UpdateWaitlistStatus validation failed")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed", Data: v.Fields})
	}

	entry := models.WaitlistEntry{ID: c.Param("id")}
	if err := entry.UpdateStatus(body.Status, body.AppointmentID); err != nil {
		log.Println("Error: UpdateWaitlistStatus failed to update waitlist entry")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to update waitlist entry"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: entry})
}

func DeleteWaitlistEntry(c echo.Context) error {
	entry := models.WaitlistEntry{ID: c.Param("id")}
	if err := entry.Delete(); err == sql.ErrNoRows {
		log.Println("Error: DeleteWaitlistEntry waitlist entry not found")
		return c.JSON(http.StatusNotFound, utils.Response{Error: "waitlist entry not found"})
	} else if err != nil {
		log.Println("Error: DeleteWaitlistEntry failed to delete")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}
