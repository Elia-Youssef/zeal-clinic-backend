package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"clinic-api/internal/monitor"
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
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load products"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: items, Total: total}})
}

func GetProductDropdown(c echo.Context) error {
	params := parseListParams(c)
	items, err := store.GetProductDropdown(params)
	if err != nil {
		log.Println("Error: [GetProductDropdown] failed to fetch product dropdown:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load options"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func GetProductByID(c echo.Context) error {
	var item store.Product
	if err := item.GetByID(c.Param("id")); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: [GetProductByID] product not found:", err)
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "Product not found"})
	} else if err != nil {
		log.Println("Error: [GetProductByID] failed to fetch product:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load product"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: item})
}

func GetProductInvoices(c echo.Context) error {
	params := parseListParams(c)
	items := store.InvoiceList{}
	total, err := items.GetByItem(c.Param("id"), "product", params)
	if err != nil {
		log.Println("Error: [GetProductInvoices] failed to fetch invoices:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load invoices"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: items, Total: total}})
}

func CreateProduct(c echo.Context) error {
	var p store.Product
	if err := c.Bind(&p); err != nil {
		log.Println("Error: [CreateProduct] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	if err := p.IsValid(); err != nil {
		log.Println("Error: [CreateProduct] validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
	}

	if err := p.Create(); err != nil {
		log.Println("Error: [CreateProduct] failed to create product:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't create product"})
	}
	monitor.CheckLowStock([]string{p.ID})
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: p})
}

func UpdateProduct(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdateProduct] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	delete(updates, "id")

	item := store.Product{ID: c.Param("id")}
	if err := item.Update(updates); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: [UpdateProduct] product not found:", err)
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "Product not found"})
	} else if err != nil {
		log.Println("Error: [UpdateProduct] failed to update product:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't update product"})
	}
	monitor.CheckLowStock([]string{item.ID})
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: item})
}

func DeleteProduct(c echo.Context) error {
	id := c.Param("id")
	if store.HasDependencies(id, map[string]string{"invoice_items": "item_id"}) {
		return c.JSON(http.StatusConflict, httpx.Response{Error: "Can't delete product while it's in use"})
	}

	item := store.Product{ID: id}
	if err := item.Delete(); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: [DeleteProduct] product not found:", err)
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "Product not found"})
	} else if err != nil {
		log.Println("Error: [DeleteProduct] failed to delete product:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't delete product"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
