package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllProductCategories(c echo.Context) error {
	params := parseListParams(c)
	items := models.ProductCategoryList{}
	total, err := items.GetAll(params)
	if err != nil {
		log.Println("Error: GetAllProductCategories failed to fetch categories:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch categories"})
	}
	if items == nil {
		items = []models.ProductCategory{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: utils.PaginatedList{Items: items, Total: total}})
}

func GetProductCategoryDropdown(c echo.Context) error {
	params := parseListParams(c)
	items, err := models.GetProductCategoryDropdown(params)
	if err != nil {
		log.Println("Error: [GetProductCategoryDropdown] failed to fetch category dropdown:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch category dropdown"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func CreateProductCategory(c echo.Context) error {
	var cat models.ProductCategory
	if err := c.Bind(&cat); err != nil {
		log.Println("Error: CreateProductCategory invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	if err := cat.IsValid(); err != nil {
		log.Println("Error: CreateProductCategory validation failed:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed"})
	}
	cat.CreatedAt = models.DateNow()
	if err := cat.Create(); err != nil {
		log.Println("Error: CreateProductCategory failed to create category:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create category"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: cat})
}

func UpdateProductCategory(c echo.Context) error {
	var updates map[string]interface{}
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: UpdateProductCategory invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	cat := models.ProductCategory{ID: c.Param("id")}
	if err := cat.Update(updates); err == sql.ErrNoRows {
		log.Println("Error: UpdateProductCategory category not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "category not found"})
	} else if err != nil {
		log.Println("Error: UpdateProductCategory failed to update category:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to update category"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: cat})
}

func DeleteProductCategory(c echo.Context) error {
	id := c.Param("id")
	if models.HasDependencies(id, map[string]string{"products": "category_id"}) {
		return c.JSON(http.StatusConflict, utils.Response{Error: "cannot delete category: has related records"})
	}

	cat := models.ProductCategory{ID: id}
	if err := cat.Delete(); err == sql.ErrNoRows {
		log.Println("Error: [DeleteProductCategory] category not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "category not found"})
	} else if err != nil {
		log.Println("Error: [DeleteProductCategory] failed to delete category:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete category"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}
