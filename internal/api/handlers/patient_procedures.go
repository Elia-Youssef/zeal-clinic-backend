package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetPatientProceduresByPatient(c echo.Context) error {
	items := store.PatientProcedureList{}
	if err := items.GetByPatient(c.Param("patientId")); err != nil {
		log.Println("Error: [GetPatientProceduresByPatient] failed to fetch patient procedures:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch patient procedures"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func GetPatientProcedureByID(c echo.Context) error {
	var pp store.PatientProcedure
	if err := pp.GetByID(c.Param("id")); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: [GetPatientProcedureByID] patient procedure not found:", err)
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "patient procedure not found"})
	} else if err != nil {
		log.Println("Error: [GetPatientProcedureByID] failed to fetch patient procedure:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch patient procedure"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: pp})
}

func CreatePatientProcedure(c echo.Context) error {
	var pp store.PatientProcedure
	if err := c.Bind(&pp); err != nil {
		log.Println("Error: [CreatePatientProcedure] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	if err := pp.IsValid(); err != nil {
		log.Println("Error: [CreatePatientProcedure] validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "validation failed"})
	}
	if err := pp.Create(); err != nil {
		log.Println("Error: [CreatePatientProcedure] failed to create patient procedure:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to create patient procedure"})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: pp})
}

func UpdatePatientProcedure(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdatePatientProcedure] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	delete(updates, "id")
	delete(updates, "patientId")
	delete(updates, "procedureId")
	pp := store.PatientProcedure{ID: c.Param("id")}
	if err := pp.Update(updates); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: [UpdatePatientProcedure] patient procedure not found:", err)
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "patient procedure not found"})
	} else if err != nil {
		log.Println("Error: [UpdatePatientProcedure] failed to update patient procedure:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to update patient procedure"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: pp})
}

func DeletePatientProcedure(c echo.Context) error {
	id := c.Param("id")
	pp := store.PatientProcedure{ID: id}
	if err := pp.Delete(); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: [DeletePatientProcedure] patient procedure not found:", err)
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "patient procedure not found"})
	} else if err != nil {
		log.Println("Error: [DeletePatientProcedure] failed to delete patient procedure:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to delete patient procedure"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
