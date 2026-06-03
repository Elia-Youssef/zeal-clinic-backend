package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllBalances(c echo.Context) error {
	params := parseListParams(c)
	entityType := c.Param("type")
	var list store.BalanceList
	total, err := list.GetAll(entityType, params)
	if err != nil {
		log.Println("Error: GetAllBalances:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load balances"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: list, Total: total}})
}

// GetEntityBalance returns the single balance row for a given entity,
// including the running totals (amount, totalIn, totalOut) that are
// maintained incrementally by each balance transaction.
func GetEntityBalance(c echo.Context) error {
	var b store.Balance
	if err := b.GetByEntityID(c.Param("type"), c.Param("id")); errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "Balance not found"})
	} else if err != nil {
		log.Println("Error: GetEntityBalance:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load balance"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: b})
}
