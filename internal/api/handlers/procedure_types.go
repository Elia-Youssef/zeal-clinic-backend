package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllProcedureTypes(c echo.Context) error {
	params := parseListParams(c)
	items := store.ProcedureTypeList{}
	total, err := items.GetAll(params)
	if err != nil {
		log.Println("Error: GetAllProcedureTypes failed to fetch types:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load types"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: items, Total: total}})
}

func GetProcedureTypeDropdown(c echo.Context) error {
	params := parseListParams(c)
	items, err := store.GetProcedureTypeDropdown(params)
	if err != nil {
		log.Println("Error: [GetProcedureTypeDropdown] failed to fetch type dropdown:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load options"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func CreateProcedureType(c echo.Context) error {
	var pt store.ProcedureType
	if err := c.Bind(&pt); err != nil {
		log.Println("Error: CreateProcedureType invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	if err := pt.IsValid(); err != nil {
		log.Println("Error: CreateProcedureType validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
	}
	if err := pt.Create(); err != nil {
		return storeError(c, err, "Type not found", "Couldn't create type")
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: pt})
}

func UpdateProcedureType(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: UpdateProcedureType invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	pt := store.ProcedureType{ID: c.Param("id")}
	if err := pt.Update(updates); err != nil {
		return storeError(c, err, "Type not found", "Couldn't update type")
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: pt})
}

func DeleteProcedureType(c echo.Context) error {
	id := c.Param("id")
	if store.HasDependencies(id, map[string]string{"procedures": "type_id"}) {
		return c.JSON(http.StatusConflict, httpx.Response{Error: "Can't delete type while it's in use"})
	}

	pt := store.ProcedureType{ID: id}
	if err := pt.Delete(); err != nil {
		return storeError(c, err, "Type not found", "Couldn't delete type")
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
