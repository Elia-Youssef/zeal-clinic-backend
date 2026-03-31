package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetPatientProceduresByPatient(c echo.Context) error {
	items, err := (&models.PatientProcedure{}).GetByPatient(c.Param("patientId"))
	if err != nil {
		log.Println("Error: [GetPatientProceduresByPatient] failed to fetch patient procedures:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch patient procedures"})
	}
	if items == nil {
		items = []models.PatientProcedure{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func GetPatientProcedureByID(c echo.Context) error {
	var pp models.PatientProcedure
	if err := pp.GetByID(c.Param("id")); err == sql.ErrNoRows {
		log.Println("Error: [GetPatientProcedureByID] patient procedure not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "patient procedure not found"})
	} else if err != nil {
		log.Println("Error: [GetPatientProcedureByID] failed to fetch patient procedure:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch patient procedure"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: pp})
}

func CreatePatientProcedure(c echo.Context) error {
	var pp models.PatientProcedure
	if err := c.Bind(&pp); err != nil {
		log.Println("Error: [CreatePatientProcedure] invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	if err := pp.IsValid(); err != nil {
		log.Println("Error: [CreatePatientProcedure] validation failed:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed", Data: err})
	}
	if err := pp.Create(); err != nil {
		log.Println("Error: [CreatePatientProcedure] failed to create patient procedure:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create patient procedure"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: pp})
}

func UpdatePatientProcedure(c echo.Context) error {
	var updates map[string]interface{}
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdatePatientProcedure] invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	delete(updates, "id")
	delete(updates, "patientId")
	delete(updates, "procedureId")
	pp := models.PatientProcedure{ID: c.Param("id")}
	if err := pp.Update(updates); err == sql.ErrNoRows {
		log.Println("Error: [UpdatePatientProcedure] patient procedure not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "patient procedure not found"})
	} else if err != nil {
		log.Println("Error: [UpdatePatientProcedure] failed to update patient procedure:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to update patient procedure"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: pp})
}

func DeletePatientProcedure(c echo.Context) error {
	id := c.Param("id")
	if models.HasDependencies(id, map[string]string{"patient_procedure_sessions": "patient_procedure_id"}) {
		return c.JSON(http.StatusConflict, utils.Response{Error: "cannot delete patient procedure: has related records"})
	}

	pp := models.PatientProcedure{ID: id}
	if err := pp.Delete(); err == sql.ErrNoRows {
		log.Println("Error: [DeletePatientProcedure] patient procedure not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "patient procedure not found"})
	} else if err != nil {
		log.Println("Error: [DeletePatientProcedure] failed to delete patient procedure:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete patient procedure"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}

