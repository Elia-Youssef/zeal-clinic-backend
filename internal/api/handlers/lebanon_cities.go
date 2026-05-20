package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"fmt"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllLebanonCities(c echo.Context) error {
	params := parseListParams(c)
	cities := store.LebanonCityList{}
	total, err := cities.GetAll(params)
	if err != nil {
		log.Println("Error: [GetAllLebanonCities] failed to fetch lebanon cities:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch lebanon cities"})
	}
	if cities == nil {
		cities = []store.LebanonCity{}
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: cities, Total: total}})
}

func GetLebanonCityDropdown(c echo.Context) error {
	params := parseListParams(c)
	cities := store.LebanonCityList{}
	_, err := cities.GetAll(params)
	if err != nil {
		log.Println("Error: [GetLebanonCityDropdown] failed to fetch lebanon city dropdown:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch lebanon city dropdown"})
	}
	items := []store.DropdownItem{}
	for _, city := range cities {
		items = append(items, store.DropdownItem{ID: city.ID, Name: fmt.Sprintf("%s, %s, %s", city.Name, city.District, city.Governorate)})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}
