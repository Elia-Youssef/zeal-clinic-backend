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
	currencies, err := (&models.Currency{}).GetAll()
	if err != nil {
		log.Println("Error: GetAllCurrencies failed to fetch currencies:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch currencies"})
	}
	if currencies == nil {
		currencies = []models.Currency{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: currencies})
}

func CreateCurrency(c echo.Context) error {
	var cur models.Currency
	if err := c.Bind(&cur); err != nil {
		log.Println("Error: CreateCurrency invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}

	if err := cur.IsValid(); err != nil {
		log.Println("Error: CreateCurrency validation failed:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed", Data: err})
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
