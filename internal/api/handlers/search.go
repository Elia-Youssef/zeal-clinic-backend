package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func SearchAll(c echo.Context) error {
	q := c.QueryParam("q")
	scopes, _ := c.Get("scopes").([]string)
	results, err := store.SearchAll(q, scopes)
	if err != nil {
		log.Println("Error: SearchAll failed:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Search failed"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: results})
}
