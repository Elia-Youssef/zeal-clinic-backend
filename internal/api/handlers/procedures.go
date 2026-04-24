package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllProcedures(c echo.Context) error {
	params := parseListParams(c)
	items := store.ProcedureList{}
	total, err := items.GetAll(params)
	if err != nil {
		log.Println("Error: [GetAllProcedures] failed to fetch procedures:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch procedures"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: items, Total: total}})
}

func GetProcedureByID(c echo.Context) error {
	var proc store.Procedure
	if err := proc.GetByID(c.Param("id")); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: [GetProcedureByID] procedure not found:", err)
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "procedure not found"})
	} else if err != nil {
		log.Println("Error: [GetProcedureByID] failed to fetch procedure:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch procedure"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: proc})
}

func GetProcedureSessions(c echo.Context) error {
	items := store.ProcedureSessionList{}
	if err := items.GetByProcedure(c.Param("id")); err != nil {
		log.Println("Error: [GetProcedureSessions] failed to fetch sessions:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch sessions"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func GetProcedurePatientProcedures(c echo.Context) error {
	items := store.PatientProcedureList{}
	if err := items.GetByProcedure(c.Param("id")); err != nil {
		log.Println("Error: [GetProcedurePatientProcedures] failed to fetch patient procedures:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch patient procedures"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func GetProcedureDropdown(c echo.Context) error {
	params := parseListParams(c)
	items, err := store.GetProcedureDropdown(params)
	if err != nil {
		log.Println("Error: [GetProcedureDropdown] failed to fetch procedure dropdown:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch procedure dropdown"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func CreateProcedure(c echo.Context) error {
	var p store.Procedure
	if err := c.Bind(&p); err != nil {
		log.Println("Error: [CreateProcedure] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	if err := p.IsValid(); err != nil {
		log.Println("Error: [CreateProcedure] validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "validation failed"})
	}
	p.IsActive = true
	if err := p.Create(); err != nil {
		log.Println("Error: [CreateProcedure] failed to create procedure:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to create procedure"})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: p})
}

func UpdateProcedure(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdateProcedure] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	delete(updates, "id")
	proc := store.Procedure{ID: c.Param("id")}
	if err := proc.Update(updates); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: [UpdateProcedure] procedure not found:", err)
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "procedure not found"})
	} else if err != nil {
		log.Println("Error: [UpdateProcedure] failed to update procedure:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to update procedure"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: proc})
}

func DeleteProcedure(c echo.Context) error {
	id := c.Param("id")
	if store.HasDependencies(id, map[string]string{"patient_procedures": "procedure_id"}) {
		return c.JSON(http.StatusConflict, httpx.Response{Error: "cannot delete procedure: has related records"})
	}

	proc := store.Procedure{ID: id}
	if err := proc.Delete(); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: [DeleteProcedure] procedure not found:", err)
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "procedure not found"})
	} else if err != nil {
		log.Println("Error: [DeleteProcedure] failed to delete procedure:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to delete procedure"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
