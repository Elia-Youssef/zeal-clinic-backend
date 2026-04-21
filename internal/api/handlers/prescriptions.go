package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetPrescriptionsByPatient(c echo.Context) error {
	list := store.PrescriptionList{}
	err := list.GetByPatient(c.Param("patientId"))
	if err != nil {
		log.Println("Error: [GetPrescriptionsByPatient] failed to fetch prescriptions:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch prescriptions"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: list})
}

func CreatePrescription(c echo.Context) error {
	var p store.Prescription
	if err := c.Bind(&p); err != nil {
		log.Println("Error: [CreatePrescription] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}

	if err := p.IsValid(); err != nil {
		log.Println("Error: [CreatePrescription] validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "validation failed"})
	}

	if err := p.Create(); err != nil {
		log.Println("Error: [CreatePrescription] failed to create prescription:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to create prescription"})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: p})
}

func UpdatePrescription(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdatePrescription] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	delete(updates, "id")
	delete(updates, "patientId")
	delete(updates, "createdAt")
	delete(updates, "updatedAt")

	p := store.Prescription{ID: c.Param("id")}
	if err := p.Update(updates); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: [UpdatePrescription] prescription not found:", err)
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "prescription not found"})
	} else if err != nil {
		log.Println("Error: [UpdatePrescription] failed to update prescription:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to update prescription"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: p})
}

func DeletePrescription(c echo.Context) error {
	p := store.Prescription{ID: c.Param("id")}
	if err := p.Delete(); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: [DeletePrescription] prescription not found:", err)
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "prescription not found"})
	} else if err != nil {
		log.Println("Error: [DeletePrescription] failed to delete prescription:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to delete prescription"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
