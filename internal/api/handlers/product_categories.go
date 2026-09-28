package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
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
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load categories"})
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
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load options"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func CreateProductCategory(c echo.Context) error {
	var cat store.ProductCategory
	if err := c.Bind(&cat); err != nil {
		log.Println("Error: CreateProductCategory invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	if err := cat.IsValid(); err != nil {
		log.Println("Error: CreateProductCategory validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
	}
	cat.CreatedAt = store.DateNow()
	if err := cat.Create(); err != nil {
		return storeError(c, err, "Category not found", "Couldn't create category")
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: cat})
}

func UpdateProductCategory(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: UpdateProductCategory invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	cat := store.ProductCategory{ID: c.Param("id")}
	if err := cat.Update(updates); err != nil {
		return storeError(c, err, "Category not found", "Couldn't update category")
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: cat})
}

func DeleteProductCategory(c echo.Context) error {
	id := c.Param("id")
	if store.HasDependencies(id, map[string]string{"products": "category_id", "product_categories": "parent_id"}) {
		return c.JSON(http.StatusConflict, httpx.Response{Error: "Can't delete category while it's in use"})
	}

	cat := store.ProductCategory{ID: id}
	if err := cat.Delete(); err != nil {
		return storeError(c, err, "Category not found", "Couldn't delete category")
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
