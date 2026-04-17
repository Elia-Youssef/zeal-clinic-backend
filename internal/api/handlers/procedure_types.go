package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllProcedureTypes(c echo.Context) error {
	params := parseListParams(c)
	items := models.ProcedureTypeList{}
	total, err := items.GetAll(params)
	if err != nil {
		log.Println("Error: GetAllProcedureTypes failed to fetch types:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch types"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: utils.PaginatedList{Items: items, Total: total}})
}

func GetProcedureTypeDropdown(c echo.Context) error {
	params := parseListParams(c)
	items, err := models.GetProcedureTypeDropdown(params)
	if err != nil {
		log.Println("Error: [GetProcedureTypeDropdown] failed to fetch type dropdown:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch type dropdown"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func CreateProcedureType(c echo.Context) error {
	var pt models.ProcedureType
	if err := c.Bind(&pt); err != nil {
		log.Println("Error: CreateProcedureType invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	if err := pt.IsValid(); err != nil {
		log.Println("Error: CreateProcedureType validation failed:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed"})
	}
	if err := pt.Create(); err != nil {
		log.Println("Error: CreateProcedureType failed to create type:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create type"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: pt})
}

func UpdateProcedureType(c echo.Context) error {
	var updates map[string]interface{}
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: UpdateProcedureType invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	pt := models.ProcedureType{ID: c.Param("id")}
	if err := pt.Update(updates); err == sql.ErrNoRows {
		log.Println("Error: UpdateProcedureType type not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "type not found"})
	} else if err != nil {
		log.Println("Error: UpdateProcedureType failed to update type:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to update type"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: pt})
}

func DeleteProcedureType(c echo.Context) error {
	id := c.Param("id")
	if models.HasDependencies(id, map[string]string{"procedures": "type_id"}) {
		return c.JSON(http.StatusConflict, utils.Response{Error: "cannot delete type: has related records"})
	}

	pt := models.ProcedureType{ID: id}
	if err := pt.Delete(); err == sql.ErrNoRows {
		log.Println("Error: [DeleteProcedureType] type not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "type not found"})
	} else if err != nil {
		log.Println("Error: [DeleteProcedureType] failed to delete type:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete type"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}
