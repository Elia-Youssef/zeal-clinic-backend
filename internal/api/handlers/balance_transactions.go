package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"log"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
)

func CreateTransaction(c echo.Context) error {
	var bt models.BalanceTransaction
	if err := c.Bind(&bt); err != nil {
		log.Println("Error: CreateTransaction invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	if err := bt.IsValid(); err != nil {
		log.Println("Error: CreateTransaction validation failed:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed", Data: err})
	}
	if err := bt.Create(); err != nil {
		log.Println("Error: CreateTransaction failed to create transaction:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create transaction: " + err.Error()})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: bt})
}

func GetBalanceTransactions(c echo.Context) error {
	items, err := (&models.BalanceTransaction{}).GetByBalanceID(c.Param("id"))
	if err != nil {
		log.Println("Error: GetBalanceTransactions failed to fetch transactions:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch transactions"})
	}
	if items == nil {
		items = []models.BalanceTransaction{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func GetAllTransactions(c echo.Context) error {
	limit := 100
	if l := c.QueryParam("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil {
			limit = n
		}
	}
	items, err := (&models.BalanceTransaction{}).GetAll(limit)
	if err != nil {
		log.Println("Error: GetAllTransactions failed to fetch transactions:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch transactions"})
	}
	if items == nil {
		items = []models.BalanceTransaction{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}
