package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllInventory(c echo.Context) error {
	items, err := (&models.InventoryItem{}).GetAll()
	if err != nil {
		log.Println("Error: GetAllInventory failed to fetch inventory")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch inventory"})
	}
	if items == nil {
		items = []models.InventoryItem{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func CreateInventoryItem(c echo.Context) error {
	var i models.InventoryItem
	if err := c.Bind(&i); err != nil {
		log.Println("Error: CreateInventoryItem invalid request")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	v := NewValidator()
	v.Required("sku", i.SKU, "SKU")
	v.Required("name", i.Name, "Name")
	v.Required("category", i.Category, "Category")
	v.OneOf("category", i.Category, []string{"Consumable", "Equipment", "Medication"}, "Category")
	v.Positive("unitPrice", i.UnitPrice, "Unit price")
	if v.HasErrors() {
		log.Println("Error: CreateInventoryItem validation failed")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed"})
	}

	if err := i.Create(); err != nil {
		log.Println("Error: CreateInventoryItem failed to create inventory item")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create inventory item"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: i})
}

func UpdateInventoryItem(c echo.Context) error {
	var updates map[string]interface{}
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: UpdateInventoryItem invalid request")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	delete(updates, "sku")
	delete(updates, "lastRestocked")

	item := models.InventoryItem{SKU: c.Param("sku")}
	if err := item.Update(updates); err == sql.ErrNoRows {
		log.Println("Error: UpdateInventoryItem inventory item not found")
		return c.JSON(http.StatusNotFound, utils.Response{Error: "inventory item not found"})
	} else if err != nil {
		log.Println("Error: UpdateInventoryItem failed to update inventory item")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to update inventory item"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: item})
}
