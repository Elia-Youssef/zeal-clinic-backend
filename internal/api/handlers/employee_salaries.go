package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetEmployeeSalaries(c echo.Context) error {
	params := parseListParams(c)
	items := store.EmployeeSalaryList{}
	total, err := items.GetByEmployee(c.Param("id"), params)
	if err != nil {
		log.Println("Error: [GetEmployeeSalaries] failed to fetch salaries:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load salaries"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: items, Total: total}})
}

func CreateEmployeeSalary(c echo.Context) error {
	var s store.EmployeeSalary
	if err := c.Bind(&s); err != nil {
		log.Println("Error: [CreateEmployeeSalary] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	s.EmployeeID = c.Param("id")
	if err := s.IsValid(); err != nil {
		log.Println("Error: [CreateEmployeeSalary] validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
	}
	if err := s.Create(); err != nil {
		log.Println("Error: [CreateEmployeeSalary] failed to create salary:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't create salary"})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: s})
}

func UpdateEmployeeSalary(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdateEmployeeSalary] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	delete(updates, "id")
	delete(updates, "employeeId")

	s := store.EmployeeSalary{ID: c.Param("id")}
	if err := s.Update(updates); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: [UpdateEmployeeSalary] salary not found:", err)
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "Salary not found"})
	} else if err != nil {
		log.Println("Error: [UpdateEmployeeSalary] failed to update salary:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't update salary"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: s})
}

func DeleteEmployeeSalary(c echo.Context) error {
	s := store.EmployeeSalary{ID: c.Param("id")}
	if err := s.Delete(); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: [DeleteEmployeeSalary] salary not found:", err)
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "Salary not found"})
	} else if err != nil {
		log.Println("Error: [DeleteEmployeeSalary] failed to delete salary:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't delete salary"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
