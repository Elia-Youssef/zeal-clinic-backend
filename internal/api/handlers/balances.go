package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
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
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch balances"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: list, Total: total}})
}
