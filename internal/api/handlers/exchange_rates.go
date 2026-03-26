package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetLatestExchangeRate(c echo.Context) error {
	from := c.QueryParam("from")
	to := c.QueryParam("to")
	if from == "" {
		from = "USD"
	}
	if to == "" {
		to = "LBP"
	}

	var rate models.ExchangeRate
	if err := rate.GetLatest(from, to); err == sql.ErrNoRows {
		return c.JSON(http.StatusOK, utils.Response{Success: true, Data: map[string]interface{}{"rate": 0, "message": "no rate set yet"}})
	} else if err != nil {
		log.Println("Error: GetLatestExchangeRate failed to fetch rate")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch rate"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: rate})
}

func GetAllExchangeRates(c echo.Context) error {
	rates, err := (&models.ExchangeRate{}).GetAll()
	if err != nil {
		log.Println("Error: GetAllExchangeRates failed to fetch rates")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch rates"})
	}
	if rates == nil {
		rates = []models.ExchangeRate{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: rates})
}

func CreateExchangeRate(c echo.Context) error {
	var rate models.ExchangeRate
	if err := c.Bind(&rate); err != nil {
		log.Println("Error: CreateExchangeRate invalid request")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}

	v := NewValidator()
	if rate.Rate <= 0 {
		v.Fields["rate"] = "rate must be greater than 0"
	}
	if v.HasErrors() {
		log.Println("Error: CreateExchangeRate validation failed")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed"})
	}

	if err := rate.Create(); err != nil {
		log.Println("Error: CreateExchangeRate failed to save rate")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to save rate"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: rate})
}
