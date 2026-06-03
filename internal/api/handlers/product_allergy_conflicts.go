package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetProductAllergyConflicts(c echo.Context) error {
	params := parseListParams(c)
	items := store.ProductAllergyConflictList{}
	total, err := items.GetByProduct(c.Param("id"), params)
	if err != nil {
		log.Println("Error: [GetProductAllergyConflicts] failed to fetch:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load product allergy conflicts"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: items, Total: total}})
}

func AddProductAllergyConflict(c echo.Context) error {
	var pac store.ProductAllergyConflict
	if err := c.Bind(&pac); err != nil {
		log.Println("Error: [AddProductAllergyConflict] invalid request body:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	pac.ProductID = c.Param("id")
	if err := pac.IsValid(); err != nil {
		log.Println("Error: [AddProductAllergyConflict] validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
	}
	pac.CreatedAt = store.DateNow()
	if err := pac.Create(); err != nil {
		log.Println("Error: [AddProductAllergyConflict] failed to add:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't add product allergy conflict"})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: pac})
}

func UpdateProductAllergyConflictNotes(c echo.Context) error {
	var pac store.ProductAllergyConflict
	if err := c.Bind(&pac); err != nil {
		log.Println("Error: [UpdateProductAllergyConflictNotes] invalid request body:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	pac.ID = c.Param("id")
	if err := pac.UpdateNotes(); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			log.Println("Error: [UpdateProductAllergyConflictNotes] not found:", c.Param("id"))
			return c.JSON(http.StatusNotFound, httpx.Response{Error: "Conflict not found"})
		}
		log.Println("Error: [UpdateProductAllergyConflictNotes] failed to update:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't update conflict"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: pac})
}

func RemoveProductAllergyConflict(c echo.Context) error {
	pac := store.ProductAllergyConflict{ID: c.Param("id")}
	if err := pac.Delete(); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			log.Println("Error: [RemoveProductAllergyConflict] not found:", c.Param("id"))
			return c.JSON(http.StatusNotFound, httpx.Response{Error: "Conflict not found"})
		}
		log.Println("Error: [RemoveProductAllergyConflict] failed to remove:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't remove conflict"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
