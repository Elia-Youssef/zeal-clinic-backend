package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllUsers(c echo.Context) error {
	params := parseListParams(c)
	users := models.UserList{}
	total, err := users.GetAll(params)
	if err != nil {
		log.Println("Error: [GetAllUsers] failed to fetch users:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch users"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: utils.PaginatedList{Items: users, Total: total}})
}

func GetUserByID(c echo.Context) error {
	var item models.User
	if err := item.GetByID(c.Param("id")); err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, utils.Response{Error: "user not found"})
	} else if err != nil {
		log.Println("Error: [GetUserByID] failed to fetch user:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch user"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: item})
}

func CreateUser(c echo.Context) error {
	var body struct {
		models.User
		Password string `json:"password"`
	}
	if err := c.Bind(&body); err != nil {
		log.Println("Error: [CreateUser] invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	body.User.IsActive = true

	if err := body.User.IsValid(); err != nil {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed"})
	}
	if body.Password == "" {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed", Data: map[string]string{"password": "Password is required"}})
	}

	hash, err := utils.HashPassword(body.Password)
	if err != nil {
		log.Println("Error: [CreateUser] failed to hash password:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create user"})
	}

	if err := body.User.Create(hash); err != nil {
		log.Println("Error: [CreateUser] failed to create user:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create user"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: body.User})
}

func UpdateUser(c echo.Context) error {
	var updates map[string]interface{}
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdateUser] invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	delete(updates, "id")

	// Handle password change separately
	var passwordHash string
	if pw, ok := updates["password"]; ok {
		delete(updates, "password")
		if pwStr, ok := pw.(string); ok && pwStr != "" {
			h, err := utils.HashPassword(pwStr)
			if err != nil {
				log.Println("Error: [UpdateUser] failed to hash password:", err)
				return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to update user"})
			}
			passwordHash = h
		}
	}

	user := models.User{ID: c.Param("id")}
	if err := user.Update(updates); err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, utils.Response{Error: "user not found"})
	} else if err != nil {
		log.Println("Error: [UpdateUser] failed to update user:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to update user"})
	}

	if passwordHash != "" {
		if err := user.UpdatePassword(passwordHash); err != nil {
			log.Println("Error: [UpdateUser] failed to update password:", err)
			return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to update password"})
		}
	}

	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: user})
}
