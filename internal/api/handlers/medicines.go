package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
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
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load medicines"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: items, Total: total}})
}

func GetMedicineDropdown(c echo.Context) error {
	params := parseListParams(c)
	items, err := store.GetMedicineDropdown(params)
	if err != nil {
		log.Println("Error: [GetMedicineDropdown] failed to fetch medicine dropdown:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load options"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func CreateMedicine(c echo.Context) error {
	var m store.Medicine
	if err := c.Bind(&m); err != nil {
		log.Println("Error: [CreateMedicine] invalid request body:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	if err := m.IsValid(); err != nil {
		log.Println("Error: [CreateMedicine] validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
	}
	if err := m.Create(); err != nil {
		return storeError(c, err, "Medicine not found", "Couldn't create medicine")
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: m})
}

func UpdateMedicine(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdateMedicine] invalid request body:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	m := store.Medicine{ID: c.Param("id")}
	if err := m.Update(updates); err != nil {
		return storeError(c, err, "Medicine not found", "Couldn't update medicine")
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: m})
}

func DeleteMedicine(c echo.Context) error {
	id := c.Param("id")
	if store.HasDependencies(id, store.MedicineDeps) {
		return c.JSON(http.StatusConflict, httpx.Response{Error: "Can't delete medicine while it's in use"})
	}

	m := store.Medicine{ID: id}
	if err := m.Delete(); err != nil {
		return storeError(c, err, "Medicine not found", "Couldn't delete medicine")
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
