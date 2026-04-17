package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllPatients(c echo.Context) error {
	params := parseListParams(c)
	patients := models.PatientList{}
	total, err := patients.GetAll(params)
	if err != nil {
		log.Println("Error: [GetAllPatients] failed to fetch patients:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch patients"})
	}
	if patients == nil {
		patients = []models.Patient{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: utils.PaginatedList{Items: patients, Total: total}})
}

func GetPatientDropdown(c echo.Context) error {
	params := parseListParams(c)
	items, err := models.GetPatientDropdown(params)
	if err != nil {
		log.Println("Error: [GetPatientDropdown] failed to fetch patient dropdown:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch patient dropdown"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
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

	if p.CountryID != "" {
		p.Country = models.Country{ID: p.CountryID}
		p.Country.GetByID()
	}

	if p.CityID != "" {
		p.City = models.LebanonCity{ID: p.CityID}
		p.City.GetByID()
	}

	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: p})
}

func CreatePatient(c echo.Context) error {
	var p models.Patient
	if err := c.Bind(&p); err != nil {
		log.Println("Error: [CreatePatient] invalid request body:", err.Error())
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	if err := p.IsValid(); err != nil {
		log.Println("Error: [CreatePatient] validation failed:", err.Error())
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed"})
	}
	p.CreatedAt = models.DateNow()

	if err := p.Create(); err != nil {
		log.Println("Error: [CreatePatient] failed to create patient:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create patient"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: p})
}

func UpdatePatient(c echo.Context) error {
	var updates map[string]interface{}
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdatePatient] invalid request body:", err.Error())
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	delete(updates, "id")
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
	id := c.Param("id")
	if models.HasDependencies(id, map[string]string{"patient_procedures": "patient_id", "appointments": "patient_id", "prescriptions": "patient_id"}) {
		return c.JSON(http.StatusConflict, utils.Response{Error: "cannot delete patient: has related records"})
	}

	p := models.Patient{ID: id}
	if err := p.Delete(); err != nil {
		if err == sql.ErrNoRows {
			log.Println("Error: [DeletePatient] patient not found:", id)
			return c.JSON(http.StatusNotFound, utils.Response{Error: "patient not found"})
		}
		log.Println("Error: [DeletePatient] failed to delete patient:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete patient"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}
