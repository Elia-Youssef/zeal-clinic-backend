package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllBalances(c echo.Context) error {
	params := parseListParams(c)
	entityType := c.Param("type")
	var list models.BalanceList
	total, err := list.GetAll(entityType, params)
	if err != nil {
		log.Println("Error: GetAllBalances:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch balances"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: utils.PaginatedList{Items: list, Total: total}})
}
