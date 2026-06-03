package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetPatientAllergies(c echo.Context) error {
	params := parseListParams(c)
	items := store.PatientAllergyList{}
	total, err := items.GetByPatient(c.Param("patientId"), params)
	if err != nil {
		log.Println("Error: [GetPatientAllergies] failed to fetch:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load patient allergies"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: items, Total: total}})
}

func AddPatientAllergy(c echo.Context) error {
	var pa store.PatientAllergy
	if err := c.Bind(&pa); err != nil {
		log.Println("Error: [AddPatientAllergy] invalid request body:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	pa.PatientID = c.Param("patientId")
	if err := pa.IsValid(); err != nil {
		log.Println("Error: [AddPatientAllergy] validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
	}
	pa.CreatedAt = store.DateNow()
	if err := pa.Create(); err != nil {
		log.Println("Error: [AddPatientAllergy] failed to add:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't add patient allergy"})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: pa})
}

func UpdatePatientAllergyNotes(c echo.Context) error {
	var pa store.PatientAllergy
	if err := c.Bind(&pa); err != nil {
		log.Println("Error: [UpdatePatientAllergyNotes] invalid request body:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	pa.ID = c.Param("id")
	if err := pa.UpdateNotes(); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			log.Println("Error: [UpdatePatientAllergyNotes] not found:", c.Param("id"))
			return c.JSON(http.StatusNotFound, httpx.Response{Error: "Patient allergy not found"})
		}
		log.Println("Error: [UpdatePatientAllergyNotes] failed to update:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't update patient allergy"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: pa})
}

func RemovePatientAllergy(c echo.Context) error {
	pa := store.PatientAllergy{ID: c.Param("id")}
	if err := pa.Delete(); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			log.Println("Error: [RemovePatientAllergy] not found:", c.Param("id"))
			return c.JSON(http.StatusNotFound, httpx.Response{Error: "Patient allergy not found"})
		}
		log.Println("Error: [RemovePatientAllergy] failed to remove:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't remove patient allergy"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
