package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllStockAdjustments(c echo.Context) error {
	adjs, err := (&models.StockAdjustment{}).GetAll()
	if err != nil {
		log.Println("Error: GetAllStockAdjustments failed to fetch stock adjustments")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch stock adjustments"})
	}
	if adjs == nil {
		adjs = []models.StockAdjustment{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: adjs})
}

func CreateStockAdjustment(c echo.Context) error {
	var a models.StockAdjustment
	if err := c.Bind(&a); err != nil {
		log.Println("Error: CreateStockAdjustment invalid request")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	v := NewValidator()
	v.Required("sku", a.SKU, "SKU")
	v.Required("type", a.Type, "Type")
	v.OneOf("type", a.Type, []string{"Purchase", "Adjustment", "Damage"}, "Type")
	v.Required("reason", a.Reason, "Reason")
	if v.HasErrors() {
		log.Println("Error: CreateStockAdjustment validation failed")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed"})
	}

	if err := a.Create(); err != nil {
		log.Println("Error: CreateStockAdjustment failed to create stock adjustment")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create stock adjustment"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: a})
}
