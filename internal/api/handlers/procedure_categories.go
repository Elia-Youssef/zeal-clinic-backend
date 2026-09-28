package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
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
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load categories"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: items, Total: total}})
}

func GetProcedureCategoryDropdown(c echo.Context) error {
	params := parseListParams(c)
	items, err := store.GetProcedureCategoryDropdown(params)
	if err != nil {
		log.Println("Error: [GetProcedureCategoryDropdown] failed to fetch category dropdown:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load options"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func CreateProcedureCategory(c echo.Context) error {
	var cat store.ProcedureCategory
	if err := c.Bind(&cat); err != nil {
		log.Println("Error: CreateProcedureCategory invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	if err := cat.IsValid(); err != nil {
		log.Println("Error: CreateProcedureCategory validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
	}
	cat.CreatedAt = store.DateNow()
	if err := cat.Create(); err != nil {
		return storeError(c, err, "Category not found", "Couldn't create category")
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: cat})
}

func UpdateProcedureCategory(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: UpdateProcedureCategory invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	cat := store.ProcedureCategory{ID: c.Param("id")}
	if err := cat.Update(updates); err != nil {
		return storeError(c, err, "Category not found", "Couldn't update category")
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: cat})
}

func DeleteProcedureCategory(c echo.Context) error {
	id := c.Param("id")
	if store.HasDependencies(id, map[string]string{"procedures": "category_id", "procedure_categories": "parent_id"}) {
		return c.JSON(http.StatusConflict, httpx.Response{Error: "Can't delete category while it's in use"})
	}

	cat := store.ProcedureCategory{ID: id}
	if err := cat.Delete(); err != nil {
		return storeError(c, err, "Category not found", "Couldn't delete category")
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
