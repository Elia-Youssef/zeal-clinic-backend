package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/auth"
	"clinic-api/internal/database/store"
	"errors"
	"log"
	"net/http"
	"slices"
	"strings"

	"github.com/labstack/echo/v4"
)

func GetAllEmployees(c echo.Context) error {
	params := parseListParams(c)
	members := store.EmployeeList{}
	total, err := members.GetAll(params)
	if err != nil {
		log.Println("Error: [GetAllEmployees] failed to fetch employees:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load employees"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: members, Total: total}})
}

func GetEmployeeDropdown(c echo.Context) error {
	params := parseListParams(c)
	items, err := store.GetEmployeeDropdown(params)
	if err != nil {
		log.Println("Error: [GetEmployeeDropdown] failed to fetch employee dropdown:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load options"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func GetEmployeeByID(c echo.Context) error {
	var item store.Employee
	if err := item.GetByID(c.Param("id")); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: [GetEmployeeByID] employee not found:", err)
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "Employee not found"})
	} else if err != nil {
		log.Println("Error: [GetEmployeeByID] failed to fetch employee:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load employee"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: item})
}

func CreateEmployee(c echo.Context) error {
	var body struct {
		store.Employee
		Username string `json:"username"`
		Password string `json:"password"`
		UserRole string `json:"userRole"`
	}
	if err := c.Bind(&body); err != nil {
		log.Println("Error: [CreateEmployee] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	m := body.Employee
	if err := m.IsValid(); err != nil {
		log.Println("Error: [CreateEmployee] validation failed:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
	}

	// Optionally create a user account for this employee
	username := strings.TrimSpace(body.Username)
	password := strings.TrimSpace(body.Password)
	if username != "" {
		heldScopes, _ := c.Get("scopes").([]string)
		if !slices.Contains(heldScopes, "users:write") {
			return c.JSON(http.StatusForbidden, httpx.Response{Error: "You don't have permission"})
		}

		userRole := strings.TrimSpace(body.UserRole)
		if userRole == "" {
			userRole = "staff"
		}
		if !slices.Contains(assignableRoles, userRole) {
			return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid role"})
		}
		var hash string
		if password != "" {
			h, err := auth.HashPassword(password)
			if err != nil {
				log.Println("Error: [CreateEmployee] failed to hash password:", err)
				return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't create user"})
			}
			hash = h
		}
		user := store.User{
			Username:    username,
			DisplayName: m.FirstName + " " + m.LastName,
			Role:        userRole,
			IsActive:    true,
		}
		if err := user.IsValid(); err != nil {
			return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check the user details"})
		}
		if err := user.Create(hash); err != nil {
			log.Println("Error: [CreateEmployee] failed to create user:", err)
			return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't create user account"})
		}
		m.UserID = &user.ID
	}

	if err := m.Create(); err != nil {
		log.Println("Error: [CreateEmployee] failed to create employee:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't create employee"})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: m})
}

func UpdateEmployee(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdateEmployee] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	delete(updates, "id")

	member := store.Employee{ID: c.Param("id")}
	if err := member.Update(updates); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: [UpdateEmployee] employee not found:", err)
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "Employee not found"})
	} else if err != nil {
		log.Println("Error: [UpdateEmployee] failed to update employee:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't update employee"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: member})
}

func DeleteEmployee(c echo.Context) error {
	id := c.Param("id")
	if store.HasDependencies(id, map[string]string{"employee_salaries": "employee_id", "employee_schedules": "employee_id", "employee_schedule_changes": "employee_id", "employee_salary_preparations": "employee_id", "prescriptions": "prescribed_by_id"}) {
		return c.JSON(http.StatusConflict, httpx.Response{Error: "Can't delete employee while it's in use"})
	}

	member := store.Employee{ID: id}
	if err := member.Delete(); errors.Is(err, store.ErrNotFound) {
		log.Println("Error: [DeleteEmployee] employee not found:", err)
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "Employee not found"})
	} else if err != nil {
		log.Println("Error: [DeleteEmployee] failed to delete employee:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't delete employee"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
