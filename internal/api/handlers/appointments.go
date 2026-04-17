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
	date := c.QueryParam("date")
	if date == "" {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "date query parameter is required"})
	}
	params := parseListParams(c)
	apts := models.AppointmentList{}
	total, err := apts.GetAll(date, params)
	if err != nil {
		log.Println("Error: GetAllAppointments failed to fetch appointments:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch appointments"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: utils.PaginatedList{Items: apts, Total: total}})
}

func GetAppointmentCountPerRoom(c echo.Context) error {
	date := c.QueryParam("date")
	if date == "" {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "date query parameter is required"})
	}
	items, err := models.GetAppointmentCountPerRoom(date)
	if err != nil {
		log.Println("Error: [GetAppointmentCountPerRoom] failed to fetch counts:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch appointment counts"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func GetPatientAppointments(c echo.Context) error {
	patientID := c.Param("patientId")
	params := parseListParams(c)
	apts := models.AppointmentList{}
	total, err := apts.GetByPatientID(patientID, params)
	if err != nil {
		log.Println("Error: GetPatientAppointments failed to fetch appointments:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch appointments"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: utils.PaginatedList{Items: apts, Total: total}})
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
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed"})
	}

	if err := a.Create(); err != nil {
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
