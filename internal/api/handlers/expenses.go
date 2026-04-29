package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"errors"
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
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch expenses"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: expenses, Total: total}})
}

func GetExpenseDropdown(c echo.Context) error {
	params := parseListParams(c)
	items, err := store.GetExpenseDropdown(params)
	if err != nil {
		log.Println("Error: [GetExpenseDropdown] failed to fetch expense dropdown:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch expense dropdown"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func GetExpenseByID(c echo.Context) error {
	var item store.Expense
	if err := item.GetByID(c.Param("id")); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: [GetExpenseByID] expense not found:", c.Param("id"))
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "expense not found"})
	} else if err != nil {
		log.Println("Error: [GetExpenseByID] failed to fetch expense:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch expense"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: item})
}

func CreateExpense(c echo.Context) error {
	var e store.Expense
	if err := c.Bind(&e); err != nil {
		log.Println("Error: [CreateExpense] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	if err := e.IsValid(); err != nil {
		log.Println("Error: [CreateExpense] validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "validation failed"})
	}

	if err := e.Create(); err != nil {
		log.Println("Error: [CreateExpense] failed to create expense:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to create expense"})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: e})
}

func UpdateExpense(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdateExpense] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	delete(updates, "id")
	delete(updates, "createdAt")

	e := store.Expense{ID: c.Param("id")}
	if err := e.Update(updates); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: [UpdateExpense] expense not found:", c.Param("id"))
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "expense not found"})
	} else if err != nil {
		log.Println("Error: [UpdateExpense] failed to update expense:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to update expense"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: e})
}

func DeleteExpense(c echo.Context) error {
	id := c.Param("id")
	if store.HasDependencies(id, map[string]string{"balances": "entity_id"}) {
		return c.JSON(http.StatusConflict, httpx.Response{Error: "cannot delete expense: has related records"})
	}

	e := store.Expense{ID: id}
	if err := e.Delete(); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: [DeleteExpense] expense not found:", id)
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "expense not found"})
	} else if err != nil {
		log.Println("Error: [DeleteExpense] failed to delete expense:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to delete expense"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
