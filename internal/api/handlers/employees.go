package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllEmployees(c echo.Context) error {
	params := parseListParams(c)
	members := models.EmployeeList{}
	total, err := members.GetAll(params)
	if err != nil {
		log.Println("Error: [GetAllEmployees] failed to fetch employees:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch employees"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: utils.PaginatedList{Items: members, Total: total}})
}

func GetEmployeeDropdown(c echo.Context) error {
	params := parseListParams(c)
	items, err := models.GetEmployeeDropdown(params)
	if err != nil {
		log.Println("Error: [GetEmployeeDropdown] failed to fetch employee dropdown:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch employee dropdown"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func GetEmployeeByID(c echo.Context) error {
	var item models.Employee
	if err := item.GetByID(c.Param("id")); err == sql.ErrNoRows {
		log.Println("Error: [GetEmployeeByID] employee not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "employee not found"})
	} else if err != nil {
		log.Println("Error: [GetEmployeeByID] failed to fetch employee:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch employee"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: item})
}

func CreateEmployee(c echo.Context) error {
	var m models.Employee
	if err := c.Bind(&m); err != nil {
		log.Println("Error: [CreateEmployee] invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	if err := m.IsValid(); err != nil {
		log.Println("Error: [CreateEmployee] validation failed:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed", Data: err})
	}

	if err := m.Create(); err != nil {
		log.Println("Error: [CreateEmployee] failed to create employee:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create employee"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: m})
}

func UpdateEmployee(c echo.Context) error {
	var updates map[string]interface{}
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdateEmployee] invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	delete(updates, "id")

	member := models.Employee{ID: c.Param("id")}
	if err := member.Update(updates); err == sql.ErrNoRows {
		log.Println("Error: [UpdateEmployee] employee not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "employee not found"})
	} else if err != nil {
		log.Println("Error: [UpdateEmployee] failed to update employee:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to update employee"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: member})
}

func DeleteEmployee(c echo.Context) error {
	id := c.Param("id")
	if models.HasDependencies(id, map[string]string{"employee_salaries": "employee_id", "schedule_availability": "employee_id"}) {
		return c.JSON(http.StatusConflict, utils.Response{Error: "cannot delete employee: has related records"})
	}

	member := models.Employee{ID: id}
	if err := member.Delete(); err == sql.ErrNoRows {
		log.Println("Error: [DeleteEmployee] employee not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "employee not found"})
	} else if err != nil {
		log.Println("Error: [DeleteEmployee] failed to delete employee:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete employee"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}
