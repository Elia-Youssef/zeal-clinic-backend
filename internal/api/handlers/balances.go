package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllBalances(c echo.Context) error {
	entityType := c.QueryParam("entityType")
	items, err := (&models.Balance{}).GetAll(entityType)
	if err != nil {
		log.Println("Error: GetAllBalances failed to fetch balances:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch balances"})
	}
	if items == nil {
		items = []models.Balance{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func GetBalanceByID(c echo.Context) error {
	var item models.Balance
	if err := item.GetByID(c.Param("id")); err == sql.ErrNoRows {
		log.Println("Error: GetBalanceByID balance not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "balance not found"})
	} else if err != nil {
		log.Println("Error: GetBalanceByID failed to fetch balance:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch balance"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: item})
}

func GetOrCreateBalance(c echo.Context) error {
	var req struct {
		EntityType string `json:"entityType"`
		EntityID   string `json:"entityId"`
		EntityName string `json:"entityName"`
		CurrencyID string `json:"currencyId"`
	}
	if err := c.Bind(&req); err != nil {
		log.Println("Error: GetOrCreateBalance invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	balance := models.Balance{
		EntityType: req.EntityType,
		EntityID:   &req.EntityID,
		EntityName: req.EntityName,
		CurrencyID: req.CurrencyID,
	}
	if err := balance.IsValid(); err != nil {
		log.Println("Error: GetOrCreateBalance validation failed:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed", Data: err})
	}
	if err := balance.GetOrCreate(); err != nil {
		log.Println("Error: GetOrCreateBalance failed to get/create balance:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to get/create balance"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: balance})
}
