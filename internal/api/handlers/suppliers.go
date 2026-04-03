package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllSuppliers(c echo.Context) error {
	params := parseListParams(c)
	suppliers := models.SupplierList{}
	total, err := suppliers.GetAll(params)
	if err != nil {
		log.Println("Error: [GetAllSuppliers] failed to fetch suppliers:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch suppliers"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: utils.PaginatedList{Items: suppliers, Total: total}})
}

func GetSupplierDropdown(c echo.Context) error {
	params := parseListParams(c)
	items, err := models.GetSupplierDropdown(params)
	if err != nil {
		log.Println("Error: [GetSupplierDropdown] failed to fetch supplier dropdown:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch supplier dropdown"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func GetSupplierByID(c echo.Context) error {
	var item models.Supplier
	if err := item.GetByID(c.Param("id")); err == sql.ErrNoRows {
		log.Println("Error: [GetSupplierByID] supplier not found:", c.Param("id"))
		return c.JSON(http.StatusNotFound, utils.Response{Error: "supplier not found"})
	} else if err != nil {
		log.Println("Error: [GetSupplierByID] failed to fetch supplier:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch supplier"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: item})
}

func CreateSupplier(c echo.Context) error {
	var s models.Supplier
	if err := c.Bind(&s); err != nil {
		log.Println("Error: [CreateSupplier] invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	if err := s.IsValid(); err != nil {
		log.Println("Error: [CreateSupplier] validation failed:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed", Data: err})
	}

	if err := s.Create(); err != nil {
		log.Println("Error: [CreateSupplier] failed to create supplier:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create supplier"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: s})
}

func UpdateSupplier(c echo.Context) error {
	var updates map[string]interface{}
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdateSupplier] invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	delete(updates, "id")
	delete(updates, "createdAt")

	s := models.Supplier{ID: c.Param("id")}
	if err := s.Update(updates); err == sql.ErrNoRows {
		log.Println("Error: [UpdateSupplier] supplier not found:", c.Param("id"))
		return c.JSON(http.StatusNotFound, utils.Response{Error: "supplier not found"})
	} else if err != nil {
		log.Println("Error: [UpdateSupplier] failed to update supplier:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to update supplier"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: s})
}

func DeleteSupplier(c echo.Context) error {
	id := c.Param("id")
	if models.HasDependencies(id, map[string]string{"balances": "entity_id"}) {
		return c.JSON(http.StatusConflict, utils.Response{Error: "cannot delete supplier: has related records"})
	}

	s := models.Supplier{ID: id}
	if err := s.Delete(); err == sql.ErrNoRows {
		log.Println("Error: [DeleteSupplier] supplier not found:", id)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "supplier not found"})
	} else if err != nil {
		log.Println("Error: [DeleteSupplier] failed to delete supplier:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete supplier"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}
