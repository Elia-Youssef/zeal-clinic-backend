package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
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
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load procedures"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: items, Total: total}})
}

func GetProcedureByID(c echo.Context) error {
	var proc store.Procedure
	if err := proc.GetByID(c.Param("id")); err != nil {
		return storeError(c, err, "Procedure not found", "Couldn't load procedure")
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: proc})
}

func GetProcedureAppointments(c echo.Context) error {
	params := parseListParams(c)
	apts := store.AppointmentList{}
	total, err := apts.GetByProcedureID(c.Param("id"), params)
	if err != nil {
		log.Println("Error: [GetProcedureAppointments] failed to fetch appointments:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load appointments"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: apts, Total: total}})
}

func GetProcedureDropdown(c echo.Context) error {
	params := parseListParams(c)
	items, err := store.GetProcedureDropdown(params)
	if err != nil {
		log.Println("Error: [GetProcedureDropdown] failed to fetch procedure dropdown:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load options"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func CreateProcedure(c echo.Context) error {
	var p store.Procedure
	if err := c.Bind(&p); err != nil {
		log.Println("Error: [CreateProcedure] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	if err := p.IsValid(); err != nil {
		log.Println("Error: [CreateProcedure] validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
	}
	p.IsActive = true
	if err := p.Create(); err != nil {
		return storeError(c, err, "Procedure not found", "Couldn't create procedure")
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: p})
}

func UpdateProcedure(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdateProcedure] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	delete(updates, "id")
	proc := store.Procedure{ID: c.Param("id")}
	if err := proc.Update(updates); err != nil {
		return storeError(c, err, "Procedure not found", "Couldn't update procedure")
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: proc})
}

func DeleteProcedure(c echo.Context) error {
	id := c.Param("id")
	if store.HasDependencies(id, map[string]string{"appointment_procedures": "procedure_id", "invoice_items": "item_id"}) {
		return c.JSON(http.StatusConflict, httpx.Response{Error: "Can't delete procedure while it's in use"})
	}

	proc := store.Procedure{ID: id}
	if err := proc.Delete(); err != nil {
		return storeError(c, err, "Procedure not found", "Couldn't delete procedure")
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
