package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllSuppliers(c echo.Context) error {
	params := parseListParams(c)
	suppliers := store.SupplierList{}
	total, err := suppliers.GetAll(params)
	if err != nil {
		log.Println("Error: [GetAllSuppliers] failed to fetch suppliers:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch suppliers"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: suppliers, Total: total}})
}

func GetSupplierDropdown(c echo.Context) error {
	params := parseListParams(c)
	items, err := store.GetSupplierDropdown(params)
	if err != nil {
		log.Println("Error: [GetSupplierDropdown] failed to fetch supplier dropdown:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch supplier dropdown"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func GetSupplierByID(c echo.Context) error {
	var item store.Supplier
	if err := item.GetByID(c.Param("id")); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: [GetSupplierByID] supplier not found:", c.Param("id"))
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "supplier not found"})
	} else if err != nil {
		log.Println("Error: [GetSupplierByID] failed to fetch supplier:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch supplier"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: item})
}

func CreateSupplier(c echo.Context) error {
	var s store.Supplier
	if err := c.Bind(&s); err != nil {
		log.Println("Error: [CreateSupplier] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	if err := s.IsValid(); err != nil {
		log.Println("Error: [CreateSupplier] validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "validation failed"})
	}

	if err := s.Create(); err != nil {
		log.Println("Error: [CreateSupplier] failed to create supplier:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to create supplier"})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: s})
}

func UpdateSupplier(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdateSupplier] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	delete(updates, "id")
	delete(updates, "createdAt")

	s := store.Supplier{ID: c.Param("id")}
	if err := s.Update(updates); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: [UpdateSupplier] supplier not found:", c.Param("id"))
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "supplier not found"})
	} else if err != nil {
		log.Println("Error: [UpdateSupplier] failed to update supplier:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to update supplier"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: s})
}

func DeleteSupplier(c echo.Context) error {
	id := c.Param("id")

	balance := store.Balance{}
	switch err := balance.GetByEntityID("supplier", id); {
	case errors.Is(err, store.ErrNotFound):
		// no balance means no dependencies
	case err != nil:
		log.Println("Error: [DeleteSupplier] failed to load balance:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to delete supplier"})
	default:
		hasDepFrom := store.HasDependencies(balance.ID, map[string]string{"invoices": "from_balance_id", "balance_transactions": "from_balance_id"})
		hasDepTo := store.HasDependencies(balance.ID, map[string]string{"invoices": "to_balance_id", "balance_transactions": "to_balance_id"})
		if hasDepFrom || hasDepTo {
			return c.JSON(http.StatusConflict, httpx.Response{Error: "cannot delete supplier: has related records"})
		}
	}

	s := store.Supplier{ID: id}
	if err := s.Delete(); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: [DeleteSupplier] supplier not found:", id)
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "supplier not found"})
	} else if err != nil {
		log.Println("Error: [DeleteSupplier] failed to delete supplier:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to delete supplier"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
