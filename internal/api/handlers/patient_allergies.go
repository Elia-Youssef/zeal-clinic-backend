package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetPatientAllergies(c echo.Context) error {
	items, err := (&models.PatientAllergy{}).GetByPatient(c.Param("patientId"))
	if err != nil {
		log.Println("Error: [GetPatientAllergies] failed to fetch:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch patient allergies"})
	}
	if items == nil {
		items = []models.PatientAllergy{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func AddPatientAllergy(c echo.Context) error {
	var pa models.PatientAllergy
	if err := c.Bind(&pa); err != nil {
		log.Println("Error: [AddPatientAllergy] invalid request body:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	pa.PatientID = c.Param("patientId")
	if err := pa.IsValid(); err != nil {
		log.Println("Error: [AddPatientAllergy] validation failed:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed", Data: err})
	}
	pa.CreatedAt = models.DateNow()
	if err := pa.Create(); err != nil {
		log.Println("Error: [AddPatientAllergy] failed to add:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to add patient allergy"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: pa})
}

func RemovePatientAllergy(c echo.Context) error {
	pa := models.PatientAllergy{ID: c.Param("id")}
	if err := pa.Delete(); err != nil {
		if err == sql.ErrNoRows {
			log.Println("Error: [RemovePatientAllergy] not found:", c.Param("id"))
			return c.JSON(http.StatusNotFound, utils.Response{Error: "patient allergy not found"})
		}
		log.Println("Error: [RemovePatientAllergy] failed to remove:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to remove patient allergy"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}
