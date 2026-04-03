package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetPatientMedicines(c echo.Context) error {
	items := models.PatientMedicineList{}
	if err := items.GetByPatient(c.Param("patientId")); err != nil {
		log.Println("Error: [GetPatientMedicines] failed to fetch:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch patient medicines"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func AddPatientMedicine(c echo.Context) error {
	var pm models.PatientMedicine
	if err := c.Bind(&pm); err != nil {
		log.Println("Error: [AddPatientMedicine] invalid request body:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	pm.PatientID = c.Param("patientId")
	if err := pm.IsValid(); err != nil {
		log.Println("Error: [AddPatientMedicine] validation failed:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed", Data: err})
	}
	pm.CreatedAt = models.DateNow()
	if err := pm.Create(); err != nil {
		log.Println("Error: [AddPatientMedicine] failed to add:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to add patient medicine"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: pm})
}

func RemovePatientMedicine(c echo.Context) error {
	pm := models.PatientMedicine{ID: c.Param("id")}
	if err := pm.Delete(); err != nil {
		if err == sql.ErrNoRows {
			log.Println("Error: [RemovePatientMedicine] not found:", c.Param("id"))
			return c.JSON(http.StatusNotFound, utils.Response{Error: "patient medicine not found"})
		}
		log.Println("Error: [RemovePatientMedicine] failed to remove:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to remove patient medicine"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}
