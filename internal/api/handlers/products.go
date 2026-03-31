package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllProducts(c echo.Context) error {
	items, err := (&models.Product{}).GetAll()
	if err != nil {
		log.Println("Error: [GetAllProducts] failed to fetch products:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch products"})
	}
	if items == nil {
		items = []models.Product{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func GetProductByID(c echo.Context) error {
	var item models.Product
	if err := item.GetByID(c.Param("id")); err == sql.ErrNoRows {
		log.Println("Error: [GetProductByID] product not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "product not found"})
	} else if err != nil {
		log.Println("Error: [GetProductByID] failed to fetch product:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch product"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: item})
}

func CreateProduct(c echo.Context) error {
	var p models.Product
	if err := c.Bind(&p); err != nil {
		log.Println("Error: [CreateProduct] invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	if err := p.IsValid(); err != nil {
		log.Println("Error: [CreateProduct] validation failed:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed", Data: err})
	}

	if err := p.Create(); err != nil {
		log.Println("Error: [CreateProduct] failed to create product:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create product"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: p})
}

func UpdateProduct(c echo.Context) error {
	var updates map[string]interface{}
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdateProduct] invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	delete(updates, "id")

	item := models.Product{ID: c.Param("id")}
	if err := item.Update(updates); err == sql.ErrNoRows {
		log.Println("Error: [UpdateProduct] product not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "product not found"})
	} else if err != nil {
		log.Println("Error: [UpdateProduct] failed to update product:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to update product"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: item})
}

func DeleteProduct(c echo.Context) error {
	id := c.Param("id")
	if models.HasDependencies(id, map[string]string{"invoice_items": "product_id"}) {
		return c.JSON(http.StatusConflict, utils.Response{Error: "cannot delete product: has related records"})
	}

	item := models.Product{ID: id}
	if err := item.Delete(); err == sql.ErrNoRows {
		log.Println("Error: [DeleteProduct] product not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "product not found"})
	} else if err != nil {
		log.Println("Error: [DeleteProduct] failed to delete product:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete product"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}
