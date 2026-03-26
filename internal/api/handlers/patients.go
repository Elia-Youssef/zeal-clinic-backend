package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

func GetAllPatients(c echo.Context) error {
	patients, err := (&models.Patient{}).GetAll()
	if err != nil {
		log.Println("Error: [GetAllPatients] failed to fetch patients:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch patients"})
	}
	if patients == nil {
		patients = []models.Patient{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: patients})
}

func GetPatientByID(c echo.Context) error {
	p := models.Patient{}
	if err := p.GetByID(c.Param("id")); err != nil {
		if err == sql.ErrNoRows {
			log.Println("Error: [GetPatientByID] patient not found:", c.Param("id"))
			return c.JSON(http.StatusNotFound, utils.Response{Error: "patient not found"})
		}
		log.Println("Error: [GetPatientByID] failed to fetch patient:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch patient"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: p})
}

func CreatePatient(c echo.Context) error {
	var p models.Patient
	if err := c.Bind(&p); err != nil {
		log.Println("Error: [CreatePatient] invalid request body")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	v := NewValidator()
	v.Required("firstName", p.FirstName, "First name")
	v.Required("lastName", p.LastName, "Last name")
	v.Required("gender", p.Gender, "Gender")
	v.OneOf("gender", p.Gender, []string{"Male", "Female"}, "Gender")
	v.Required("dateOfBirth", p.DateOfBirth, "Date of birth")
	v.Date("dateOfBirth", p.DateOfBirth)
	v.Required("contact", p.Contact, "Contact phone")
	v.Phone("contact", p.Contact)
	v.Email("email", p.Email)
	if v.HasErrors() {
		log.Println("Error: [CreatePatient] validation failed")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed", Data: v.Fields})
	}
	p.CreatedAt = time.Now().Format(time.RFC3339)

	if err := p.Create(); err != nil {
		log.Println("Error: [CreatePatient] failed to create patient:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create patient"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: p})
}

func UpdatePatient(c echo.Context) error {
	var updates map[string]interface{}
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdatePatient] invalid request body")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	delete(updates, "id")
	delete(updates, "patientNumber")
	delete(updates, "createdAt")

	p := models.Patient{ID: c.Param("id")}
	if err := p.Update(updates); err != nil {
		if err == sql.ErrNoRows {
			log.Println("Error: [UpdatePatient] patient not found:", c.Param("id"))
			return c.JSON(http.StatusNotFound, utils.Response{Error: "patient not found"})
		}
		log.Println("Error: [UpdatePatient] failed to update patient:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to update patient"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: p})
}

func DeletePatient(c echo.Context) error {
	p := models.Patient{ID: c.Param("id")}
	if err := p.Delete(); err != nil {
		if err == sql.ErrNoRows {
			log.Println("Error: [DeletePatient] patient not found:", c.Param("id"))
			return c.JSON(http.StatusNotFound, utils.Response{Error: "patient not found"})
		}
		log.Println("Error: [DeletePatient] failed to delete patient:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete patient"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}
