package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

func GetAllAppointments(c echo.Context) error {
	apts, err := (&models.Appointment{}).GetAll()
	if err != nil {
		log.Println("Error: GetAllAppointments failed to fetch appointments:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch appointments"})
	}
	if apts == nil {
		apts = []models.Appointment{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: apts})
}

func CreateAppointment(c echo.Context) error {
	var a models.Appointment
	if err := c.Bind(&a); err != nil {
		log.Println("Error: CreateAppointment invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	if a.Status == "" {
		a.Status = "Scheduled"
	}
	if err := a.IsValid(); err != nil {
		log.Println("Error: CreateAppointment validation failed:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed", Data: err})
	}

	if err := a.Create(); err != nil {
		if strings.Contains(err.Error(), "room conflict") {
			log.Println("Error: CreateAppointment room conflict:", err)
			return c.JSON(http.StatusConflict, utils.Response{Error: err.Error()})
		}
		log.Println("Error: CreateAppointment failed to create appointment:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create appointment"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: a})
}

func UpdateAppointment(c echo.Context) error {
	var updates map[string]interface{}
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: UpdateAppointment invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	delete(updates, "id")

	apt := models.Appointment{ID: c.Param("id")}
	if err := apt.Update(updates); err != nil {
		if err == sql.ErrNoRows {
			log.Println("Error: UpdateAppointment appointment not found:", err)
			return c.JSON(http.StatusNotFound, utils.Response{Error: "appointment not found"})
		}
		if strings.Contains(err.Error(), "room conflict") {
			log.Println("Error: UpdateAppointment room conflict:", err)
			return c.JSON(http.StatusConflict, utils.Response{Error: err.Error()})
		}
		log.Println("Error: UpdateAppointment failed to update appointment:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to update appointment"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: apt})
}

func DeleteAppointment(c echo.Context) error {
	id := c.Param("id")
	if models.HasDependencies(id, map[string]string{"bookings": "appointment_id"}) {
		return c.JSON(http.StatusConflict, utils.Response{Error: "cannot delete appointment: has related records"})
	}

	apt := models.Appointment{ID: id}
	if err := apt.Delete(); err != nil {
		if err == sql.ErrNoRows {
			log.Println("Error: [DeleteAppointment] appointment not found:", err)
			return c.JSON(http.StatusNotFound, utils.Response{Error: "appointment not found"})
		}
		log.Println("Error: [DeleteAppointment] failed to delete appointment:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete appointment"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}
