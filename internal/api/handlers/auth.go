package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

func Login(c echo.Context) error {
	var req models.LoginRequest
	if err := c.Bind(&req); err != nil {
		log.Println("Error: [Login] invalid request body")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}

	username := strings.TrimSpace(req.Username)
	password := strings.TrimSpace(req.Password)
	if username == "" || password == "" {
		log.Println("Error: [Login] missing username or password")
		return c.JSON(http.StatusUnauthorized, utils.Response{Error: "username and password required"})
	}

	user := models.User{}
	if err := user.GetByUsername(username); err != nil {
		if err == sql.ErrNoRows || strings.Contains(err.Error(), "no rows") {
			log.Println("Error: [Login] user not found:", username)
			return c.JSON(http.StatusUnauthorized, utils.Response{Error: "invalid username or password"})
		}
		log.Println("Error: [Login] db error:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "authentication failed"})
	}

	if !user.IsActive {
		log.Println("Error: [Login] account disabled:", username)
		return c.JSON(http.StatusForbidden, utils.Response{Error: "account is disabled"})
	}

	match, err := utils.ComparePassword(password, user.PasswordHash)
	if err != nil || !match {
		log.Println("Error: [Login] invalid password for:", username)
		return c.JSON(http.StatusUnauthorized, utils.Response{Error: "invalid username or password"})
	}

	token, err := utils.GenerateToken(user)
	if err != nil {
		log.Println("Error: [Login] token generation failed:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to generate token"})
	}

	return c.JSON(http.StatusOK, utils.Response{
		Success: true,
		Data: models.LoginResponse{
			Token: token,
			User:  user.DisplayName,
			Role:  user.Role,
		},
	})
}

func Logout(c echo.Context) error {
	auth := c.Request().Header.Get("Authorization")
	if tokenStr, ok := strings.CutPrefix(auth, "Bearer "); ok {
		t := models.Token{Token: tokenStr}
		t.Delete()
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}
