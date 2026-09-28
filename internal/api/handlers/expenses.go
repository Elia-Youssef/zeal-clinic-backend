package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllExpenses(c echo.Context) error {
	params := parseListParams(c)
	expenses := store.ExpenseList{}
	total, err := expenses.GetAll(params)
	if err != nil {
		log.Println("Error: [GetAllExpenses] failed to fetch expenses:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load expenses"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: expenses, Total: total}})
}

func GetExpenseDropdown(c echo.Context) error {
	params := parseListParams(c)
	items, err := store.GetExpenseDropdown(params)
	if err != nil {
		log.Println("Error: [GetExpenseDropdown] failed to fetch expense dropdown:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load options"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func GetExpenseByID(c echo.Context) error {
	var item store.Expense
	if err := item.GetByID(c.Param("id")); err != nil {
		return storeError(c, err, "Expense not found", "Couldn't load expense")
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: item})
}

func CreateExpense(c echo.Context) error {
	var e store.Expense
	if err := c.Bind(&e); err != nil {
		log.Println("Error: [CreateExpense] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	if err := e.IsValid(); err != nil {
		log.Println("Error: [CreateExpense] validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
	}

	if err := e.Create(); err != nil {
		return storeError(c, err, "Expense not found", "Couldn't create expense")
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: e})
}

func UpdateExpense(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdateExpense] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	delete(updates, "id")
	delete(updates, "createdAt")

	e := store.Expense{ID: c.Param("id")}
	if err := e.Update(updates); err != nil {
		return storeError(c, err, "Expense not found", "Couldn't update expense")
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: e})
}

func DeleteExpense(c echo.Context) error {
	id := c.Param("id")
	e := store.Expense{ID: id}
	if err := e.Delete(); err != nil {
		return storeError(c, err, "Expense not found", "Couldn't delete expense")
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
