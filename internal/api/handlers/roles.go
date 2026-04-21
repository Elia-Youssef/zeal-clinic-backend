package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllRoles(c echo.Context) error {
	params := parseListParams(c)
	roles := store.RoleList{}
	total, err := roles.GetAll(params)
	if err != nil {
		log.Println("Error: [GetAllRoles] failed to fetch roles:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch roles"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: roles, Total: total}})
}

func GetRoleDropdown(c echo.Context) error {
	params := parseListParams(c)
	items, err := store.GetRoleDropdown(params)
	if err != nil {
		log.Println("Error: [GetRoleDropdown] failed to fetch role dropdown:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch role dropdown"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func GetRoleByName(c echo.Context) error {
	role := store.Role{}
	if err := role.GetByName(c.Param("name")); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			log.Println("Error: [GetRoleByName] role not found:", c.Param("name"))
			return c.JSON(http.StatusNotFound, httpx.Response{Error: "role not found"})
		}
		log.Println("Error: [GetRoleByName] failed to fetch role:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch role"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: role})
}

func UpdateRole(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdateRole] invalid request body:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	delete(updates, "name")

	role := store.Role{Name: c.Param("name")}
	if err := role.Update(updates); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			log.Println("Error: [UpdateRole] role not found:", c.Param("name"))
			return c.JSON(http.StatusNotFound, httpx.Response{Error: "role not found"})
		}
		log.Println("Error: [UpdateRole] failed to update role:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to update role"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: role})
}
