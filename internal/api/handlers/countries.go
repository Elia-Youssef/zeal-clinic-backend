package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllCountries(c echo.Context) error {
	params := parseListParams(c)
	countries := store.CountryList{}
	total, err := countries.GetAll(params)
	if err != nil {
		log.Println("Error: [GetAllCountries] failed to fetch countries:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch countries"})
	}
	if countries == nil {
		countries = []store.Country{}
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: countries, Total: total}})
}

func GetCountryDropdown(c echo.Context) error {
	params := parseListParams(c)
	items, err := store.GetCountryDropdown(params)
	if err != nil {
		log.Println("Error: [GetCountryDropdown] failed to fetch country dropdown:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch country dropdown"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}
