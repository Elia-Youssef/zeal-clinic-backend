package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetPatientMedicines(c echo.Context) error {
	params := parseListParams(c)
	items := store.PatientMedicineList{}
	total, err := items.GetByPatient(c.Param("patientId"), params)
	if err != nil {
		log.Println("Error: [GetPatientMedicines] failed to fetch:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load patient medicines"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: items, Total: total}})
}

func AddPatientMedicine(c echo.Context) error {
	var pm store.PatientMedicine
	if err := c.Bind(&pm); err != nil {
		log.Println("Error: [AddPatientMedicine] invalid request body:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	pm.PatientID = c.Param("patientId")
	if err := pm.IsValid(); err != nil {
		log.Println("Error: [AddPatientMedicine] validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
	}
	pm.CreatedAt = store.DateNow()
	if err := pm.Create(); err != nil {
		return storeError(c, err, "Patient medicine not found", "Couldn't add patient medicine")
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: pm})
}

func UpdatePatientMedicineNotes(c echo.Context) error {
	var pm store.PatientMedicine
	if err := c.Bind(&pm); err != nil {
		log.Println("Error: [UpdatePatientMedicineNotes] invalid request body:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	pm.ID = c.Param("id")
	if err := pm.UpdateNotes(); err != nil {
		return storeError(c, err, "Patient medicine not found", "Couldn't update patient medicine")
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: pm})
}

func RemovePatientMedicine(c echo.Context) error {
	pm := store.PatientMedicine{ID: c.Param("id")}
	if err := pm.Delete(); err != nil {
		return storeError(c, err, "Patient medicine not found", "Couldn't remove patient medicine")
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
