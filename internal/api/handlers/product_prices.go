package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetProductPrices(c echo.Context) error {
	params := parseListParams(c)
	items := store.ProductPriceList{}
	total, err := items.GetByProduct(c.Param("id"), params)
	if err != nil {
		log.Println("Error: [GetProductPrices] failed to fetch prices:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch prices"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: items, Total: total}})
}
