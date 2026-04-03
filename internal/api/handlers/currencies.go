package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllCurrencies(c echo.Context) error {
	params := parseListParams(c)
	currencies := models.CurrencyList{}
	total, err := currencies.GetAll(params)
	if err != nil {
		log.Println("Error: GetAllCurrencies failed to fetch currencies:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch currencies"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: utils.PaginatedList{Items: currencies, Total: total}})
}

func GetCurrencyDropdown(c echo.Context) error {
	params := parseListParams(c)
	items, err := models.GetCurrencyDropdown(params)
	if err != nil {
		log.Println("Error: [GetCurrencyDropdown] failed to fetch currency dropdown:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch currency dropdown"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func CreateCurrency(c echo.Context) error {
	var cur models.Currency
	if err := c.Bind(&cur); err != nil {
		log.Println("Error: CreateCurrency invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}

	if err := cur.IsValid(); err != nil {
		log.Println("Error: CreateCurrency validation failed:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed"})
	}

	if err := cur.Create(); err != nil {
		log.Println("Error: CreateCurrency failed to save currency:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to save currency"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: cur})
}

func UpdateCurrency(c echo.Context) error {
	var updates map[string]interface{}
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: UpdateCurrency invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	delete(updates, "id")

	cur := models.Currency{ID: c.Param("id")}
	if err := cur.Update(updates); err != nil {
		if err == sql.ErrNoRows {
			log.Println("Error: UpdateCurrency currency not found:", err)
			return c.JSON(http.StatusNotFound, utils.Response{Error: "currency not found"})
		}
		log.Println("Error: UpdateCurrency failed to update currency:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to update currency"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: cur})
}

func DeleteCurrency(c echo.Context) error {
	id := c.Param("id")
	if models.HasDependencies(id, map[string]string{"balances": "currency_id"}) {
		return c.JSON(http.StatusConflict, utils.Response{Error: "cannot delete currency: has related records"})
	}

	cur := models.Currency{ID: id}
	if err := cur.Delete(); err != nil {
		if err == sql.ErrNoRows {
			log.Println("Error: [DeleteCurrency] currency not found:", err)
			return c.JSON(http.StatusNotFound, utils.Response{Error: "currency not found"})
		}
		log.Println("Error: [DeleteCurrency] failed to delete currency:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete currency"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}
