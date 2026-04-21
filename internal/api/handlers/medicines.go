package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllMedicines(c echo.Context) error {
	params := parseListParams(c)
	items := store.MedicineList{}
	total, err := items.GetAll(params)
	if err != nil {
		log.Println("Error: [GetAllMedicines] failed to fetch medicines:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch medicines"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: items, Total: total}})
}

func GetMedicineDropdown(c echo.Context) error {
	params := parseListParams(c)
	items, err := store.GetMedicineDropdown(params)
	if err != nil {
		log.Println("Error: [GetMedicineDropdown] failed to fetch medicine dropdown:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch medicine dropdown"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func CreateMedicine(c echo.Context) error {
	var m store.Medicine
	if err := c.Bind(&m); err != nil {
		log.Println("Error: [CreateMedicine] invalid request body:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	if err := m.IsValid(); err != nil {
		log.Println("Error: [CreateMedicine] validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "validation failed"})
	}
	if err := m.Create(); err != nil {
		log.Println("Error: [CreateMedicine] failed to create medicine:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to create medicine"})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: m})
}

func UpdateMedicine(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdateMedicine] invalid request body:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	m := store.Medicine{ID: c.Param("id")}
	if err := m.Update(updates); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			log.Println("Error: [UpdateMedicine] medicine not found:", c.Param("id"))
			return c.JSON(http.StatusNotFound, httpx.Response{Error: "medicine not found"})
		}
		log.Println("Error: [UpdateMedicine] failed to update medicine:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to update medicine"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: m})
}

func DeleteMedicine(c echo.Context) error {
	id := c.Param("id")
	if store.HasDependencies(id, store.MedicineDeps) {
		return c.JSON(http.StatusConflict, httpx.Response{Error: "cannot delete medicine: has related records"})
	}

	m := store.Medicine{ID: id}
	if err := m.Delete(); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			log.Println("Error: [DeleteMedicine] medicine not found:", id)
			return c.JSON(http.StatusNotFound, httpx.Response{Error: "medicine not found"})
		}
		log.Println("Error: [DeleteMedicine] failed to delete medicine:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to delete medicine"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
