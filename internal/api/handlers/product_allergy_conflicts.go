package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetProductAllergyConflicts(c echo.Context) error {
	items := models.ProductAllergyConflictList{}
	err := items.GetByProduct(c.Param("id"))
	if err != nil {
		log.Println("Error: [GetProductAllergyConflicts] failed to fetch:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch product allergy conflicts"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func AddProductAllergyConflict(c echo.Context) error {
	var pac models.ProductAllergyConflict
	if err := c.Bind(&pac); err != nil {
		log.Println("Error: [AddProductAllergyConflict] invalid request body:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	pac.ProductID = c.Param("id")
	if err := pac.IsValid(); err != nil {
		log.Println("Error: [AddProductAllergyConflict] validation failed:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed"})
	}
	pac.CreatedAt = models.DateNow()
	if err := pac.Create(); err != nil {
		log.Println("Error: [AddProductAllergyConflict] failed to add:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to add product allergy conflict"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: pac})
}

func RemoveProductAllergyConflict(c echo.Context) error {
	pac := models.ProductAllergyConflict{ID: c.Param("id")}
	if err := pac.Delete(); err != nil {
		if err == sql.ErrNoRows {
			log.Println("Error: [RemoveProductAllergyConflict] not found:", c.Param("id"))
			return c.JSON(http.StatusNotFound, utils.Response{Error: "conflict not found"})
		}
		log.Println("Error: [RemoveProductAllergyConflict] failed to remove:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to remove conflict"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}
