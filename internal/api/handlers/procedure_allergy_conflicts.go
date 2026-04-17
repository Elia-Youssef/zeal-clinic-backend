package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetProcedureAllergyConflicts(c echo.Context) error {
	items := models.ProcedureAllergyConflictList{}
	err := items.GetByProcedure(c.Param("procedureId"))
	if err != nil {
		log.Println("Error: [GetProcedureAllergyConflicts] failed to fetch:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch procedure allergy conflicts"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func AddProcedureAllergyConflict(c echo.Context) error {
	var pac models.ProcedureAllergyConflict
	if err := c.Bind(&pac); err != nil {
		log.Println("Error: [AddProcedureAllergyConflict] invalid request body:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	pac.ProcedureID = c.Param("procedureId")
	if err := pac.IsValid(); err != nil {
		log.Println("Error: [AddProcedureAllergyConflict] validation failed:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed"})
	}
	pac.CreatedAt = models.DateNow()
	if err := pac.Create(); err != nil {
		log.Println("Error: [AddProcedureAllergyConflict] failed to add:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to add procedure allergy conflict"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: pac})
}

func RemoveProcedureAllergyConflict(c echo.Context) error {
	pac := models.ProcedureAllergyConflict{ID: c.Param("id")}
	if err := pac.Delete(); err != nil {
		if err == sql.ErrNoRows {
			log.Println("Error: [RemoveProcedureAllergyConflict] not found:", c.Param("id"))
			return c.JSON(http.StatusNotFound, utils.Response{Error: "conflict not found"})
		}
		log.Println("Error: [RemoveProcedureAllergyConflict] failed to remove:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to remove conflict"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}
