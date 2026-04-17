package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllCountries(c echo.Context) error {
	params := parseListParams(c)
	countries := models.CountryList{}
	total, err := countries.GetAll(params)
	if err != nil {
		log.Println("Error: [GetAllCountries] failed to fetch countries:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch countries"})
	}
	if countries == nil {
		countries = []models.Country{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: utils.PaginatedList{Items: countries, Total: total}})
}

func GetCountryDropdown(c echo.Context) error {
	params := parseListParams(c)
	items, err := models.GetCountryDropdown(params)
	if err != nil {
		log.Println("Error: [GetCountryDropdown] failed to fetch country dropdown:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch country dropdown"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}
