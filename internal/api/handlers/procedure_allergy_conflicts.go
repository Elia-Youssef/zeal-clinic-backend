package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetProcedureAllergyConflicts(c echo.Context) error {
	params := parseListParams(c)
	items := store.ProcedureAllergyConflictList{}
	total, err := items.GetByProcedure(c.Param("procedureId"), params)
	if err != nil {
		log.Println("Error: [GetProcedureAllergyConflicts] failed to fetch:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load procedure allergy conflicts"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: items, Total: total}})
}

func AddProcedureAllergyConflict(c echo.Context) error {
	var pac store.ProcedureAllergyConflict
	if err := c.Bind(&pac); err != nil {
		log.Println("Error: [AddProcedureAllergyConflict] invalid request body:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	pac.ProcedureID = c.Param("procedureId")
	if err := pac.IsValid(); err != nil {
		log.Println("Error: [AddProcedureAllergyConflict] validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
	}
	pac.CreatedAt = store.DateNow()
	if err := pac.Create(); err != nil {
		return storeError(c, err, "Conflict not found", "Couldn't add procedure allergy conflict")
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: pac})
}

func UpdateProcedureAllergyConflictNotes(c echo.Context) error {
	var pac store.ProcedureAllergyConflict
	if err := c.Bind(&pac); err != nil {
		log.Println("Error: [UpdateProcedureAllergyConflictNotes] invalid request body:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	pac.ID = c.Param("id")
	if err := pac.UpdateNotes(); err != nil {
		return storeError(c, err, "Conflict not found", "Couldn't update conflict")
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: pac})
}

func RemoveProcedureAllergyConflict(c echo.Context) error {
	pac := store.ProcedureAllergyConflict{ID: c.Param("id")}
	if err := pac.Delete(); err != nil {
		return storeError(c, err, "Conflict not found", "Couldn't remove conflict")
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
