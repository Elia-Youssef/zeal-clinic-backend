package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

func GetAllProductCategories(c echo.Context) error {
	items, err := (&models.ProductCategory{}).GetAll()
	if err != nil {
		log.Println("Error: GetAllProductCategories failed to fetch categories")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch categories"})
	}
	if items == nil {
		items = []models.ProductCategory{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func CreateProductCategory(c echo.Context) error {
	var cat models.ProductCategory
	if err := c.Bind(&cat); err != nil {
		log.Println("Error: CreateProductCategory invalid request")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	v := NewValidator()
	v.Required("name", cat.Name, "Name")
	if v.HasErrors() {
		log.Println("Error: CreateProductCategory validation failed")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed"})
	}
	cat.CreatedAt = time.Now().Format(time.RFC3339)
	if err := cat.Create(); err != nil {
		log.Println("Error: CreateProductCategory failed to create category")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create category"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: cat})
}

func UpdateProductCategory(c echo.Context) error {
	var updates map[string]interface{}
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: UpdateProductCategory invalid request")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	cat := models.ProductCategory{ID: c.Param("id")}
	if err := cat.Update(updates); err == sql.ErrNoRows {
		log.Println("Error: UpdateProductCategory category not found")
		return c.JSON(http.StatusNotFound, utils.Response{Error: "category not found"})
	} else if err != nil {
		log.Println("Error: UpdateProductCategory failed to update category")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to update category"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: cat})
}

func DeleteProductCategory(c echo.Context) error {
	cat := models.ProductCategory{ID: c.Param("id")}
	if err := cat.Delete(); err == sql.ErrNoRows {
		log.Println("Error: DeleteProductCategory category not found")
		return c.JSON(http.StatusNotFound, utils.Response{Error: "category not found"})
	} else if err != nil {
		log.Println("Error: DeleteProductCategory failed to delete category")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete category"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: "deleted"})
}
