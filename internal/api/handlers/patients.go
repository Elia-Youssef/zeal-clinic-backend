package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"clinic-api/internal/validation"
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllPatients(c echo.Context) error {
	params := parseListParams(c)
	patients := store.PatientList{}
	total, err := patients.GetAll(params)
	if err != nil {
		log.Println("Error: [GetAllPatients] failed to fetch patients:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load patients"})
	}
	if patients == nil {
		patients = []store.Patient{}
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: patients, Total: total}})
}

func GetPatientDropdown(c echo.Context) error {
	params := parseListParams(c)
	items, err := store.GetPatientDropdown(params)
	if err != nil {
		log.Println("Error: [GetPatientDropdown] failed to fetch patient dropdown:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load options"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func GetPatientByID(c echo.Context) error {
	p := store.Patient{}
	if err := p.GetByID(c.Param("id")); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			log.Println("Error: [GetPatientByID] patient not found:", c.Param("id"))
			return c.JSON(http.StatusNotFound, httpx.Response{Error: "Patient not found"})
		}
		log.Println("Error: [GetPatientByID] failed to fetch patient:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load patient"})
	}

	if p.CountryID != "" {
		p.Country = store.Country{ID: p.CountryID}
		p.Country.GetByID()
	}

	if p.CityID != "" {
		p.City = store.LebanonCity{ID: p.CityID}
		p.City.GetByID()
	}

	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: p})
}

func CreatePatient(c echo.Context) error {
	var p store.Patient
	if err := c.Bind(&p); err != nil {
		log.Println("Error: [CreatePatient] invalid request body:", err.Error())
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	if err := p.IsValid(); err != nil {
		log.Println("Error: [CreatePatient] validation failed:", err.Error())
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
	}
	p.CreatedAt = store.DateNow()

	if err := p.Create(); err != nil {
		log.Println("Error: [CreatePatient] failed to create patient:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't create patient"})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: p})
}

func UpdatePatient(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdatePatient] invalid request body:", err.Error())
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	delete(updates, "id")
	delete(updates, "createdAt")

	p := store.Patient{ID: c.Param("id")}
	if err := p.Update(updates); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			log.Println("Error: [UpdatePatient] patient not found:", c.Param("id"))
			return c.JSON(http.StatusNotFound, httpx.Response{Error: "Patient not found"})
		}
		var validationErr validation.Errors
		if errors.As(err, &validationErr) {
			log.Println("Error: [UpdatePatient] validation failed:", err.Error())
			return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
		}
		log.Println("Error: [UpdatePatient] failed to update patient:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't update patient"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: p})
}

func DeletePatient(c echo.Context) error {
	id := c.Param("id")
	if store.HasDependencies(id, map[string]string{"appointment_procedures": "patient_id", "appointments": "patient_id", "prescriptions": "patient_id"}) {
		return c.JSON(http.StatusConflict, httpx.Response{Error: "Can't delete patient while it's in use"})
	}

	p := store.Patient{ID: id}
	if err := p.Delete(); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			log.Println("Error: [DeletePatient] patient not found:", id)
			return c.JSON(http.StatusNotFound, httpx.Response{Error: "Patient not found"})
		}
		log.Println("Error: [DeletePatient] failed to delete patient:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't delete patient"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
