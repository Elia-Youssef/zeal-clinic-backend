package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetProcedurePrices(c echo.Context) error {
	params := parseListParams(c)
	items := store.ProcedurePriceList{}
	total, err := items.GetByProcedure(c.Param("id"), params)
	if err != nil {
		log.Println("Error: [GetProcedurePrices] failed to fetch prices:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load prices"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: items, Total: total}})
}
