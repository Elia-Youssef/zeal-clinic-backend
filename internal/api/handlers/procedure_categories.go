package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllProcedureCategories(c echo.Context) error {
	params := parseListParams(c)
	items := store.ProcedureCategoryList{}
	total, err := items.GetAll(params)
	if err != nil {
		log.Println("Error: GetAllProcedureCategories failed to fetch categories:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch categories"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: items, Total: total}})
}

func GetProcedureCategoryDropdown(c echo.Context) error {
	params := parseListParams(c)
	items, err := store.GetProcedureCategoryDropdown(params)
	if err != nil {
		log.Println("Error: [GetProcedureCategoryDropdown] failed to fetch category dropdown:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch category dropdown"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func CreateProcedureCategory(c echo.Context) error {
	var cat store.ProcedureCategory
	if err := c.Bind(&cat); err != nil {
		log.Println("Error: CreateProcedureCategory invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	if err := cat.IsValid(); err != nil {
		log.Println("Error: CreateProcedureCategory validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "validation failed"})
	}
	cat.CreatedAt = store.DateNow()
	if err := cat.Create(); err != nil {
		log.Println("Error: CreateProcedureCategory failed to create category:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to create category"})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: cat})
}

func UpdateProcedureCategory(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: UpdateProcedureCategory invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	cat := store.ProcedureCategory{ID: c.Param("id")}
	if err := cat.Update(updates); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: UpdateProcedureCategory category not found:", err)
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "category not found"})
	} else if err != nil {
		log.Println("Error: UpdateProcedureCategory failed to update category:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to update category"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: cat})
}

func DeleteProcedureCategory(c echo.Context) error {
	id := c.Param("id")
	if store.HasDependencies(id, map[string]string{"procedures": "category_id"}) {
		return c.JSON(http.StatusConflict, httpx.Response{Error: "cannot delete category: has related records"})
	}

	cat := store.ProcedureCategory{ID: id}
	if err := cat.Delete(); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: [DeleteProcedureCategory] category not found:", err)
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "category not found"})
	} else if err != nil {
		log.Println("Error: [DeleteProcedureCategory] failed to delete category:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to delete category"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
