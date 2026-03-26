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
		log.Println("Error: GetAllAppointments failed to fetch appointments")
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
		log.Println("Error: CreateAppointment invalid request")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	v := NewValidator()
	v.Required("patientId", a.PatientID, "Patient ID")
	v.Required("roomId", a.RoomID, "Room ID")
	v.Required("startTime", a.StartTime, "Start time")
	v.DateTime("startTime", a.StartTime)
	v.Required("endTime", a.EndTime, "End time")
	v.DateTime("endTime", a.EndTime)
	v.Required("treatmentType", a.TreatmentType, "Treatment type")
	v.OneOf("treatmentType", a.TreatmentType, []string{"Consultation", "Procedure", "Follow-up"}, "Treatment type")
	if v.HasErrors() {
		log.Println("Error: CreateAppointment validation failed")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed"})
	}
	if a.Status == "" {
		a.Status = "Scheduled"
	}
	v.OneOf("status", a.Status, []string{"Scheduled", "In-Progress", "Completed", "Cancelled"}, "Status")
	if v.HasErrors() {
		log.Println("Error: CreateAppointment validation failed")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed"})
	}

	if err := a.Create(); err != nil {
		if strings.Contains(err.Error(), "room conflict") {
			log.Println("Error: CreateAppointment room conflict")
			return c.JSON(http.StatusConflict, utils.Response{Error: err.Error()})
		}
		log.Println("Error: CreateAppointment failed to create appointment")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create appointment"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: a})
}

func UpdateAppointment(c echo.Context) error {
	var updates map[string]interface{}
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: UpdateAppointment invalid request")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	delete(updates, "id")

	apt := models.Appointment{ID: c.Param("id")}
	if err := apt.Update(updates); err != nil {
		if err == sql.ErrNoRows {
			log.Println("Error: UpdateAppointment appointment not found")
			return c.JSON(http.StatusNotFound, utils.Response{Error: "appointment not found"})
		}
		if strings.Contains(err.Error(), "room conflict") {
			log.Println("Error: UpdateAppointment room conflict")
			return c.JSON(http.StatusConflict, utils.Response{Error: err.Error()})
		}
		log.Println("Error: UpdateAppointment failed to update appointment")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to update appointment"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: apt})
}

func DeleteAppointment(c echo.Context) error {
	apt := models.Appointment{ID: c.Param("id")}
	if err := apt.Delete(); err != nil {
		if err == sql.ErrNoRows {
			log.Println("Error: DeleteAppointment appointment not found")
			return c.JSON(http.StatusNotFound, utils.Response{Error: "appointment not found"})
		}
		log.Println("Error: DeleteAppointment failed to delete appointment")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete appointment"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}

// MarkAppointmentReminded toggles the reminder_sent status on an appointment.
func MarkAppointmentReminded(c echo.Context) error {
	var body struct {
		Sent bool `json:"sent"`
	}
	if err := c.Bind(&body); err != nil {
		log.Println("Error: MarkAppointmentReminded invalid request")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}

	apt := models.Appointment{ID: c.Param("id")}
	if err := apt.MarkReminded(body.Sent); err != nil {
		if err == sql.ErrNoRows {
			log.Println("Error: MarkAppointmentReminded appointment not found")
			return c.JSON(http.StatusNotFound, utils.Response{Error: "appointment not found"})
		}
		log.Println("Error: MarkAppointmentReminded failed to update reminder status")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to update reminder status"})
	}

	return c.JSON(http.StatusOK, utils.Response{Success: true})
}

// ApproveAppointment marks a consultation appointment as approved.
func ApproveAppointment(c echo.Context) error {
	var body struct {
		ApprovedBy string `json:"approvedBy"`
	}
	if err := c.Bind(&body); err != nil {
		log.Println("Error: ApproveAppointment invalid request")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	apt := models.Appointment{ID: c.Param("id")}
	if err := apt.Approve(body.ApprovedBy); err != nil {
		if err == sql.ErrNoRows {
			log.Println("Error: ApproveAppointment appointment not found")
			return c.JSON(http.StatusNotFound, utils.Response{Error: "appointment not found"})
		}
		log.Println("Error: ApproveAppointment failed to approve appointment")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to approve appointment"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: apt})
}

// RejectAppointment marks a consultation appointment as rejected.
func RejectAppointment(c echo.Context) error {
	var body struct {
		ApprovedBy string `json:"approvedBy"`
	}
	if err := c.Bind(&body); err != nil {
		log.Println("Error: RejectAppointment invalid request")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	apt := models.Appointment{ID: c.Param("id")}
	if err := apt.Reject(body.ApprovedBy); err != nil {
		if err == sql.ErrNoRows {
			log.Println("Error: RejectAppointment appointment not found")
			return c.JSON(http.StatusNotFound, utils.Response{Error: "appointment not found"})
		}
		log.Println("Error: RejectAppointment failed to reject appointment")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to reject appointment"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: apt})
}
