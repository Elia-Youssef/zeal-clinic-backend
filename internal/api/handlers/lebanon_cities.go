package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"fmt"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllLebanonCities(c echo.Context) error {
	params := parseListParams(c)
	cities := models.LebanonCityList{}
	total, err := cities.GetAll(params)
	if err != nil {
		log.Println("Error: [GetAllLebanonCities] failed to fetch lebanon cities:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch lebanon cities"})
	}
	if cities == nil {
		cities = []models.LebanonCity{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: utils.PaginatedList{Items: cities, Total: total}})
}

func GetLebanonCityDropdown(c echo.Context) error {
	params := parseListParams(c)
	cities := models.LebanonCityList{}
	_, err := cities.GetAll(params)
	if err != nil {
		log.Println("Error: [GetLebanonCityDropdown] failed to fetch lebanon city dropdown:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch lebanon city dropdown"})
	}
	items := []models.DropdownItem{}
	for _, city := range cities {
		items = append(items, models.DropdownItem{ID: city.ID, Name: fmt.Sprintf("%s, %s, %s", city.Governorate, city.District, city.Name)})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}
