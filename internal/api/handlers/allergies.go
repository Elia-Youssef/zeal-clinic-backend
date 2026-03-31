package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllAllergies(c echo.Context) error {
	items, err := (&models.Allergy{}).GetAll()
	if err != nil {
		log.Println("Error: [GetAllAllergies] failed to fetch allergies:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch allergies"})
	}
	if items == nil {
		items = []models.Allergy{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func CreateAllergy(c echo.Context) error {
	var a models.Allergy
	if err := c.Bind(&a); err != nil {
		log.Println("Error: [CreateAllergy] invalid request body:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	if err := a.IsValid(); err != nil {
		log.Println("Error: [CreateAllergy] validation failed:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed", Data: err})
	}
	a.CreatedAt = models.DateNow()
	if err := a.Create(); err != nil {
		log.Println("Error: [CreateAllergy] failed to create allergy:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create allergy"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: a})
}

func UpdateAllergy(c echo.Context) error {
	var updates map[string]interface{}
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdateAllergy] invalid request body:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	a := models.Allergy{ID: c.Param("id")}
	if err := a.Update(updates); err != nil {
		if err == sql.ErrNoRows {
			log.Println("Error: [UpdateAllergy] allergy not found:", c.Param("id"))
			return c.JSON(http.StatusNotFound, utils.Response{Error: "allergy not found"})
		}
		log.Println("Error: [UpdateAllergy] failed to update allergy:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to update allergy"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: a})
}

func DeleteAllergy(c echo.Context) error {
	id := c.Param("id")
	if models.HasDependencies(id, map[string]string{"patient_allergies": "allergy_id", "procedure_allergy_conflicts": "allergy_id", "product_allergy_conflicts": "allergy_id"}) {
		return c.JSON(http.StatusConflict, utils.Response{Error: "cannot delete allergy: has related records"})
	}

	a := models.Allergy{ID: id}
	if err := a.Delete(); err != nil {
		if err == sql.ErrNoRows {
			log.Println("Error: [DeleteAllergy] allergy not found:", id)
			return c.JSON(http.StatusNotFound, utils.Response{Error: "allergy not found"})
		}
		log.Println("Error: [DeleteAllergy] failed to delete allergy:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete allergy"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}

