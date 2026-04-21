package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllProductCategories(c echo.Context) error {
	params := parseListParams(c)
	items := store.ProductCategoryList{}
	total, err := items.GetAll(params)
	if err != nil {
		log.Println("Error: GetAllProductCategories failed to fetch categories:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch categories"})
	}
	if items == nil {
		items = []store.ProductCategory{}
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: items, Total: total}})
}

func GetProductCategoryDropdown(c echo.Context) error {
	params := parseListParams(c)
	items, err := store.GetProductCategoryDropdown(params)
	if err != nil {
		log.Println("Error: [GetProductCategoryDropdown] failed to fetch category dropdown:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch category dropdown"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func CreateProductCategory(c echo.Context) error {
	var cat store.ProductCategory
	if err := c.Bind(&cat); err != nil {
		log.Println("Error: CreateProductCategory invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	if err := cat.IsValid(); err != nil {
		log.Println("Error: CreateProductCategory validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "validation failed"})
	}
	cat.CreatedAt = store.DateNow()
	if err := cat.Create(); err != nil {
		log.Println("Error: CreateProductCategory failed to create category:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to create category"})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: cat})
}

func UpdateProductCategory(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: UpdateProductCategory invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	cat := store.ProductCategory{ID: c.Param("id")}
	if err := cat.Update(updates); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: UpdateProductCategory category not found:", err)
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "category not found"})
	} else if err != nil {
		log.Println("Error: UpdateProductCategory failed to update category:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to update category"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: cat})
}

func DeleteProductCategory(c echo.Context) error {
	id := c.Param("id")
	if store.HasDependencies(id, map[string]string{"products": "category_id"}) {
		return c.JSON(http.StatusConflict, httpx.Response{Error: "cannot delete category: has related records"})
	}

	cat := store.ProductCategory{ID: id}
	if err := cat.Delete(); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: [DeleteProductCategory] category not found:", err)
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "category not found"})
	} else if err != nil {
		log.Println("Error: [DeleteProductCategory] failed to delete category:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to delete category"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
