package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllMedicines(c echo.Context) error {
	params := parseListParams(c)
	items := models.MedicineList{}
	total, err := items.GetAll(params)
	if err != nil {
		log.Println("Error: [GetAllMedicines] failed to fetch medicines:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch medicines"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: utils.PaginatedList{Items: items, Total: total}})
}

func GetMedicineDropdown(c echo.Context) error {
	params := parseListParams(c)
	items, err := models.GetMedicineDropdown(params)
	if err != nil {
		log.Println("Error: [GetMedicineDropdown] failed to fetch medicine dropdown:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch medicine dropdown"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func CreateMedicine(c echo.Context) error {
	var m models.Medicine
	if err := c.Bind(&m); err != nil {
		log.Println("Error: [CreateMedicine] invalid request body:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	if err := m.IsValid(); err != nil {
		log.Println("Error: [CreateMedicine] validation failed:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed"})
	}
	if err := m.Create(); err != nil {
		log.Println("Error: [CreateMedicine] failed to create medicine:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create medicine"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: m})
}

func UpdateMedicine(c echo.Context) error {
	var updates map[string]interface{}
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdateMedicine] invalid request body:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	m := models.Medicine{ID: c.Param("id")}
	if err := m.Update(updates); err != nil {
		if err == sql.ErrNoRows {
			log.Println("Error: [UpdateMedicine] medicine not found:", c.Param("id"))
			return c.JSON(http.StatusNotFound, utils.Response{Error: "medicine not found"})
		}
		log.Println("Error: [UpdateMedicine] failed to update medicine:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to update medicine"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: m})
}

func DeleteMedicine(c echo.Context) error {
	id := c.Param("id")
	if models.HasDependencies(id, models.MedicineDeps) {
		return c.JSON(http.StatusConflict, utils.Response{Error: "cannot delete medicine: has related records"})
	}

	m := models.Medicine{ID: id}
	if err := m.Delete(); err != nil {
		if err == sql.ErrNoRows {
			log.Println("Error: [DeleteMedicine] medicine not found:", id)
			return c.JSON(http.StatusNotFound, utils.Response{Error: "medicine not found"})
		}
		log.Println("Error: [DeleteMedicine] failed to delete medicine:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete medicine"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}
