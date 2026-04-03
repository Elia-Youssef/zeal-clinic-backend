package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetEmployeeSalaries(c echo.Context) error {
	items := models.EmployeeSalaryList{}
	if err := items.GetByEmployee(c.Param("id")); err != nil {
		log.Println("Error: [GetEmployeeSalaries] failed to fetch salaries:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch salaries"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func CreateEmployeeSalary(c echo.Context) error {
	var s models.EmployeeSalary
	if err := c.Bind(&s); err != nil {
		log.Println("Error: [CreateEmployeeSalary] invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	s.EmployeeID = c.Param("id")
	if err := s.IsValid(); err != nil {
		log.Println("Error: [CreateEmployeeSalary] validation failed:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed", Data: err})
	}
	if err := s.Create(); err != nil {
		log.Println("Error: [CreateEmployeeSalary] failed to create salary:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create salary"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: s})
}

func UpdateEmployeeSalary(c echo.Context) error {
	var updates map[string]interface{}
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdateEmployeeSalary] invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	delete(updates, "id")
	delete(updates, "employeeId")

	s := models.EmployeeSalary{ID: c.Param("id")}
	if err := s.Update(updates); err == sql.ErrNoRows {
		log.Println("Error: [UpdateEmployeeSalary] salary not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "salary not found"})
	} else if err != nil {
		log.Println("Error: [UpdateEmployeeSalary] failed to update salary:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to update salary"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: s})
}

func DeleteEmployeeSalary(c echo.Context) error {
	s := models.EmployeeSalary{ID: c.Param("id")}
	if err := s.Delete(); err == sql.ErrNoRows {
		log.Println("Error: [DeleteEmployeeSalary] salary not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "salary not found"})
	} else if err != nil {
		log.Println("Error: [DeleteEmployeeSalary] failed to delete salary:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete salary"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}
