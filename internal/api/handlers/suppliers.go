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
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load suppliers"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: suppliers, Total: total}})
}

func GetSupplierDropdown(c echo.Context) error {
	params := parseListParams(c)
	items, err := store.GetSupplierDropdown(params)
	if err != nil {
		log.Println("Error: [GetSupplierDropdown] failed to fetch supplier dropdown:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load options"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func GetSupplierByID(c echo.Context) error {
	var item store.Supplier
	if err := item.GetByID(c.Param("id")); err != nil {
		return storeError(c, err, "Supplier not found", "Couldn't load supplier")
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: item})
}

func CreateSupplier(c echo.Context) error {
	var s store.Supplier
	if err := c.Bind(&s); err != nil {
		log.Println("Error: [CreateSupplier] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	if err := s.IsValid(); err != nil {
		log.Println("Error: [CreateSupplier] validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
	}

	if err := s.Create(); err != nil {
		return storeError(c, err, "Supplier not found", "Couldn't create supplier")
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: s})
}

func UpdateSupplier(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdateSupplier] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	delete(updates, "id")
	delete(updates, "createdAt")

	s := store.Supplier{ID: c.Param("id")}
	if err := s.Update(updates); err != nil {
		return storeError(c, err, "Supplier not found", "Couldn't update supplier")
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
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't delete supplier"})
	default:
		hasDepFrom := store.HasDependencies(balance.ID, map[string]string{"invoices": "from_balance_id", "balance_transactions": "from_balance_id"})
		hasDepTo := store.HasDependencies(balance.ID, map[string]string{"invoices": "to_balance_id", "balance_transactions": "to_balance_id"})
		if hasDepFrom || hasDepTo {
			return c.JSON(http.StatusConflict, httpx.Response{Error: "Can't delete supplier while it's in use"})
		}
	}

	s := store.Supplier{ID: id}
	if err := s.Delete(); err != nil {
		return storeError(c, err, "Supplier not found", "Couldn't delete supplier")
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
