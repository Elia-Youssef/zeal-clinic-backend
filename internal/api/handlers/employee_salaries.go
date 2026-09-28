package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
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
		return storeError(c, err, "Salary not found", "Couldn't create salary")
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
	if err := s.Update(updates); err != nil {
		return storeError(c, err, "Salary not found", "Couldn't update salary")
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: s})
}

func DeleteEmployeeSalary(c echo.Context) error {
	s := store.EmployeeSalary{ID: c.Param("id")}
	if store.HasDependencies(s.ID, map[string]string{"employee_salary_preparations": "salary_id"}) {
		return c.JSON(http.StatusConflict, httpx.Response{Error: "Can't delete salary while it's in use"})
	}
	if err := s.Delete(); err != nil {
		return storeError(c, err, "Salary not found", "Couldn't delete salary")
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
