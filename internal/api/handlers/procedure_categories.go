package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllProcedureCategories(c echo.Context) error {
	params := parseListParams(c)
	items := models.ProcedureCategoryList{}
	total, err := items.GetAll(params)
	if err != nil {
		log.Println("Error: GetAllProcedureCategories failed to fetch categories:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch categories"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: utils.PaginatedList{Items: items, Total: total}})
}

func GetProcedureCategoryDropdown(c echo.Context) error {
	params := parseListParams(c)
	items, err := models.GetProcedureCategoryDropdown(params)
	if err != nil {
		log.Println("Error: [GetProcedureCategoryDropdown] failed to fetch category dropdown:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch category dropdown"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func CreateProcedureCategory(c echo.Context) error {
	var cat models.ProcedureCategory
	if err := c.Bind(&cat); err != nil {
		log.Println("Error: CreateProcedureCategory invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	if err := cat.IsValid(); err != nil {
		log.Println("Error: CreateProcedureCategory validation failed:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed"})
	}
	cat.CreatedAt = models.DateNow()
	if err := cat.Create(); err != nil {
		log.Println("Error: CreateProcedureCategory failed to create category:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create category"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: cat})
}

func UpdateProcedureCategory(c echo.Context) error {
	var updates map[string]interface{}
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: UpdateProcedureCategory invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	cat := models.ProcedureCategory{ID: c.Param("id")}
	if err := cat.Update(updates); err == sql.ErrNoRows {
		log.Println("Error: UpdateProcedureCategory category not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "category not found"})
	} else if err != nil {
		log.Println("Error: UpdateProcedureCategory failed to update category:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to update category"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: cat})
}

func DeleteProcedureCategory(c echo.Context) error {
	id := c.Param("id")
	if models.HasDependencies(id, map[string]string{"procedures": "category_id"}) {
		return c.JSON(http.StatusConflict, utils.Response{Error: "cannot delete category: has related records"})
	}

	cat := models.ProcedureCategory{ID: id}
	if err := cat.Delete(); err == sql.ErrNoRows {
		log.Println("Error: [DeleteProcedureCategory] category not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "category not found"})
	} else if err != nil {
		log.Println("Error: [DeleteProcedureCategory] failed to delete category:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete category"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}
