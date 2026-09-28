package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetPrescriptionsByPatient(c echo.Context) error {
	params := parseListParams(c)
	list := store.PrescriptionList{}
	total, err := list.GetByPatient(c.Param("patientId"), params)
	if err != nil {
		log.Println("Error: [GetPrescriptionsByPatient] failed to fetch prescriptions:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load prescriptions"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: list, Total: total}})
}

func CreatePrescription(c echo.Context) error {
	var p store.Prescription
	if err := c.Bind(&p); err != nil {
		log.Println("Error: [CreatePrescription] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}

	if err := p.IsValid(); err != nil {
		log.Println("Error: [CreatePrescription] validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
	}

	if err := p.Create(); err != nil {
		return storeError(c, err, "Prescription not found", "Couldn't create prescription")
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: p})
}

func UpdatePrescription(c echo.Context) error {
	var p store.Prescription
	if err := c.Bind(&p); err != nil {
		log.Println("Error: [UpdatePrescription] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	p.ID = c.Param("id")

	if err := p.IsValid(); err != nil {
		log.Println("Error: [UpdatePrescription] validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
	}

	if err := p.Update(); err != nil {
		return storeError(c, err, "Prescription not found", "Couldn't update prescription")
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: p})
}

func DeletePrescription(c echo.Context) error {
	p := store.Prescription{ID: c.Param("id")}
	if err := p.Delete(); err != nil {
		return storeError(c, err, "Prescription not found", "Couldn't delete prescription")
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
