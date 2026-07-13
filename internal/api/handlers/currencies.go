package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllCurrencies(c echo.Context) error {
	params := parseListParams(c)
	currencies := store.CurrencyList{}
	total, err := currencies.GetAll(params)
	if err != nil {
		log.Println("Error: GetAllCurrencies failed to fetch currencies:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load currencies"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: currencies, Total: total}})
}

func GetCurrencyDropdown(c echo.Context) error {
	params := parseListParams(c)
	items, err := store.GetCurrencyDropdown(params)
	if err != nil {
		log.Println("Error: [GetCurrencyDropdown] failed to fetch currency dropdown:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load options"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func CreateCurrency(c echo.Context) error {
	var cur store.Currency
	if err := c.Bind(&cur); err != nil {
		log.Println("Error: CreateCurrency invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}

	if err := cur.IsValid(); err != nil {
		log.Println("Error: CreateCurrency validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
	}

	if err := cur.Create(); err != nil {
		log.Println("Error: CreateCurrency failed to save currency:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't save currency"})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: cur})
}

func UpdateCurrency(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: UpdateCurrency invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	delete(updates, "id")
	delete(updates, "code")

	cur := store.Currency{ID: c.Param("id")}
	if err := cur.Update(updates); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			log.Println("Error: UpdateCurrency currency not found:", err)
			return c.JSON(http.StatusNotFound, httpx.Response{Error: "Currency not found"})
		}
		log.Println("Error: UpdateCurrency failed to update currency:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't update currency"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: cur})
}

func DeleteCurrency(c echo.Context) error {
	id := c.Param("id")
	if store.HasDependencies(id, map[string]string{"balances": "currency_id"}) {
		return c.JSON(http.StatusConflict, httpx.Response{Error: "Can't delete currency while it's in use"})
	}

	cur := store.Currency{ID: id}
	if err := cur.Delete(); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			log.Println("Error: [DeleteCurrency] currency not found:", err)
			return c.JSON(http.StatusNotFound, httpx.Response{Error: "Currency not found"})
		}
		log.Println("Error: [DeleteCurrency] failed to delete currency:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't delete currency"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
