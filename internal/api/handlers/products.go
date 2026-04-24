package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllProducts(c echo.Context) error {
	params := parseListParams(c)
	items := store.ProductList{}
	total, err := items.GetAll(params)
	if err != nil {
		log.Println("Error: [GetAllProducts] failed to fetch products:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch products"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: items, Total: total}})
}

func GetProductDropdown(c echo.Context) error {
	params := parseListParams(c)
	items, err := store.GetProductDropdown(params)
	if err != nil {
		log.Println("Error: [GetProductDropdown] failed to fetch product dropdown:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch product dropdown"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func GetProductByID(c echo.Context) error {
	var item store.Product
	if err := item.GetByID(c.Param("id")); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: [GetProductByID] product not found:", err)
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "product not found"})
	} else if err != nil {
		log.Println("Error: [GetProductByID] failed to fetch product:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch product"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: item})
}

func GetProductInvoices(c echo.Context) error {
	items := store.InvoiceList{}
	if err := items.GetByItem(c.Param("id"), "product"); err != nil {
		log.Println("Error: [GetProductInvoices] failed to fetch invoices:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch invoices"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func CreateProduct(c echo.Context) error {
	var p store.Product
	if err := c.Bind(&p); err != nil {
		log.Println("Error: [CreateProduct] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	if err := p.IsValid(); err != nil {
		log.Println("Error: [CreateProduct] validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "validation failed"})
	}

	if err := p.Create(); err != nil {
		log.Println("Error: [CreateProduct] failed to create product:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to create product"})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: p})
}

func UpdateProduct(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdateProduct] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	delete(updates, "id")

	item := store.Product{ID: c.Param("id")}
	if err := item.Update(updates); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: [UpdateProduct] product not found:", err)
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "product not found"})
	} else if err != nil {
		log.Println("Error: [UpdateProduct] failed to update product:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to update product"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: item})
}

func DeleteProduct(c echo.Context) error {
	id := c.Param("id")
	if store.HasDependencies(id, map[string]string{"invoice_items": "product_id"}) {
		return c.JSON(http.StatusConflict, httpx.Response{Error: "cannot delete product: has related records"})
	}

	item := store.Product{ID: id}
	if err := item.Delete(); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: [DeleteProduct] product not found:", err)
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "product not found"})
	} else if err != nil {
		log.Println("Error: [DeleteProduct] failed to delete product:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to delete product"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
