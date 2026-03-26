package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllRoles(c echo.Context) error {
	roles, err := (&models.Role{}).GetAll()
	if err != nil {
		log.Println("Error: [GetAllRoles] failed to fetch roles:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch roles"})
	}
	if roles == nil {
		roles = []models.Role{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: roles})
}

func GetRoleByName(c echo.Context) error {
	role := models.Role{}
	if err := role.GetByName(c.Param("name")); err != nil {
		if err == sql.ErrNoRows {
			log.Println("Error: [GetRoleByName] role not found:", c.Param("name"))
			return c.JSON(http.StatusNotFound, utils.Response{Error: "role not found"})
		}
		log.Println("Error: [GetRoleByName] failed to fetch role:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch role"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: role})
}

func UpdateRole(c echo.Context) error {
	var updates map[string]interface{}
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdateRole] invalid request body")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	delete(updates, "name")

	role := models.Role{Name: c.Param("name")}
	if err := role.Update(updates); err != nil {
		if err == sql.ErrNoRows {
			log.Println("Error: [UpdateRole] role not found:", c.Param("name"))
			return c.JSON(http.StatusNotFound, utils.Response{Error: "role not found"})
		}
		log.Println("Error: [UpdateRole] failed to update role:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to update role"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: role})
}
