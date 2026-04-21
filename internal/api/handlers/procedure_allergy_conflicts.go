package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetProcedureAllergyConflicts(c echo.Context) error {
	items := store.ProcedureAllergyConflictList{}
	err := items.GetByProcedure(c.Param("procedureId"))
	if err != nil {
		log.Println("Error: [GetProcedureAllergyConflicts] failed to fetch:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch procedure allergy conflicts"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func AddProcedureAllergyConflict(c echo.Context) error {
	var pac store.ProcedureAllergyConflict
	if err := c.Bind(&pac); err != nil {
		log.Println("Error: [AddProcedureAllergyConflict] invalid request body:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	pac.ProcedureID = c.Param("procedureId")
	if err := pac.IsValid(); err != nil {
		log.Println("Error: [AddProcedureAllergyConflict] validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "validation failed"})
	}
	pac.CreatedAt = store.DateNow()
	if err := pac.Create(); err != nil {
		log.Println("Error: [AddProcedureAllergyConflict] failed to add:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to add procedure allergy conflict"})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: pac})
}

func RemoveProcedureAllergyConflict(c echo.Context) error {
	pac := store.ProcedureAllergyConflict{ID: c.Param("id")}
	if err := pac.Delete(); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			log.Println("Error: [RemoveProcedureAllergyConflict] not found:", c.Param("id"))
			return c.JSON(http.StatusNotFound, httpx.Response{Error: "conflict not found"})
		}
		log.Println("Error: [RemoveProcedureAllergyConflict] failed to remove:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to remove conflict"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
