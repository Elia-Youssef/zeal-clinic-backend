package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllAllergies(c echo.Context) error {
	params := parseListParams(c)
	items := store.AllergyList{}
	total, err := items.GetAll(params)
	if err != nil {
		log.Println("Error: [GetAllAllergies] failed to fetch allergies:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load allergies"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: items, Total: total}})
}

func GetAllergyDropdown(c echo.Context) error {
	params := parseListParams(c)
	items, err := store.GetAllergyDropdown(params)
	if err != nil {
		log.Println("Error: [GetAllergyDropdown] failed to fetch allergy dropdown:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load options"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func CreateAllergy(c echo.Context) error {
	var a store.Allergy
	if err := c.Bind(&a); err != nil {
		log.Println("Error: [CreateAllergy] invalid request body:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	if err := a.IsValid(); err != nil {
		log.Println("Error: [CreateAllergy] validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
	}
	if err := a.Create(); err != nil {
		log.Println("Error: [CreateAllergy] failed to create allergy:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't create allergy"})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: a})
}

func UpdateAllergy(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdateAllergy] invalid request body:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	a := store.Allergy{ID: c.Param("id")}
	if err := a.Update(updates); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			log.Println("Error: [UpdateAllergy] allergy not found:", c.Param("id"))
			return c.JSON(http.StatusNotFound, httpx.Response{Error: "Allergy not found"})
		}
		log.Println("Error: [UpdateAllergy] failed to update allergy:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't update allergy"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: a})
}

func DeleteAllergy(c echo.Context) error {
	id := c.Param("id")
	if store.HasDependencies(id, store.AllergyDeps) {
		return c.JSON(http.StatusConflict, httpx.Response{Error: "Can't delete allergy while it's in use"})
	}

	a := store.Allergy{ID: id}
	if err := a.Delete(); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			log.Println("Error: [DeleteAllergy] allergy not found:", id)
			return c.JSON(http.StatusNotFound, httpx.Response{Error: "Allergy not found"})
		}
		log.Println("Error: [DeleteAllergy] failed to delete allergy:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't delete allergy"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
