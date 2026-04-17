package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllProcedures(c echo.Context) error {
	params := parseListParams(c)
	items := models.ProcedureList{}
	total, err := items.GetAll(params)
	if err != nil {
		log.Println("Error: [GetAllProcedures] failed to fetch procedures:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch procedures"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: utils.PaginatedList{Items: items, Total: total}})
}

func GetProcedureByID(c echo.Context) error {
	var proc models.Procedure
	if err := proc.GetByID(c.Param("id")); err == sql.ErrNoRows {
		log.Println("Error: [GetProcedureByID] procedure not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "procedure not found"})
	} else if err != nil {
		log.Println("Error: [GetProcedureByID] failed to fetch procedure:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch procedure"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: proc})
}

func GetProcedureDropdown(c echo.Context) error {
	params := parseListParams(c)
	items, err := models.GetProcedureDropdown(params)
	if err != nil {
		log.Println("Error: [GetProcedureDropdown] failed to fetch procedure dropdown:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch procedure dropdown"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func CreateProcedure(c echo.Context) error {
	var p models.Procedure
	if err := c.Bind(&p); err != nil {
		log.Println("Error: [CreateProcedure] invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	if err := p.IsValid(); err != nil {
		log.Println("Error: [CreateProcedure] validation failed:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed"})
	}
	p.IsActive = true
	if err := p.Create(); err != nil {
		log.Println("Error: [CreateProcedure] failed to create procedure:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create procedure"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: p})
}

func UpdateProcedure(c echo.Context) error {
	var updates map[string]interface{}
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdateProcedure] invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	delete(updates, "id")
	proc := models.Procedure{ID: c.Param("id")}
	if err := proc.Update(updates); err == sql.ErrNoRows {
		log.Println("Error: [UpdateProcedure] procedure not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "procedure not found"})
	} else if err != nil {
		log.Println("Error: [UpdateProcedure] failed to update procedure:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to update procedure"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: proc})
}

func DeleteProcedure(c echo.Context) error {
	id := c.Param("id")
	if models.HasDependencies(id, map[string]string{"patient_procedures": "procedure_id"}) {
		return c.JSON(http.StatusConflict, utils.Response{Error: "cannot delete procedure: has related records"})
	}

	proc := models.Procedure{ID: id}
	if err := proc.Delete(); err == sql.ErrNoRows {
		log.Println("Error: [DeleteProcedure] procedure not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "procedure not found"})
	} else if err != nil {
		log.Println("Error: [DeleteProcedure] failed to delete procedure:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete procedure"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}
