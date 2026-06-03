package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/auth"
	"clinic-api/internal/database/store"
	"clinic-api/internal/tracking"
	"clinic-api/internal/validation"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (r *LoginRequest) IsValid() error {
	e := make(validation.Errors)
	if strings.TrimSpace(r.Username) == "" {
		e["username"] = "username is required"
	}
	if strings.TrimSpace(r.Password) == "" {
		e["password"] = "password is required"
	}
	if len(e) > 0 {
		return e
	}
	return nil
}

func Login(c echo.Context) error {
	var req LoginRequest
	if err := c.Bind(&req); err != nil {
		log.Println("Error: [Login] invalid request body")
		tracking.Warn(c, "[Login] invalid request body")
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}

	if err := req.IsValid(); err != nil {
		log.Println("Error: [Login] missing username or password")
		tracking.Warn(c, "[Login] missing username or password")
		return c.JSON(http.StatusUnauthorized, httpx.Response{Error: "Enter your username and password"})
	}

	username := strings.TrimSpace(req.Username)
	password := strings.TrimSpace(req.Password)

	user := store.User{}
	if err := user.GetByUsername(username); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			log.Println("Error: [Login] user not found:", username)
			tracking.Warn(c, "[Login] user not found: "+username)
			return c.JSON(http.StatusUnauthorized, httpx.Response{Error: "Invalid username or password"})
		}
		log.Println("Error: [Login] db error:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't sign you in"})
	}

	if !user.IsActive {
		log.Println("Error: [Login] account disabled:", username)
		tracking.Warn(c, "[Login] account disabled: "+username)
		return c.JSON(http.StatusForbidden, httpx.Response{Error: "Your account is disabled"})
	}

	if user.PasswordHash == "" {
		hash, err := auth.HashPassword(password)
		if err != nil {
			log.Println("Error: [Login] failed to hash initial password:", err)
			return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't sign you in"})
		}
		if err := user.UpdatePassword(hash); err != nil {
			log.Println("Error: [Login] failed to set initial password:", err)
			return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't sign you in"})
		}
		user.PasswordHash = hash
	} else {
		match, err := auth.ComparePassword(password, user.PasswordHash)
		if err != nil || !match {
			log.Println("Error: [Login] invalid password for:", username)
			tracking.Warn(c, "[Login] invalid password for: "+username)
			return c.JSON(http.StatusUnauthorized, httpx.Response{Error: "Invalid username or password"})
		}
	}

	role := store.Role{}
	if err := role.GetByName(user.Role); err != nil {
		log.Println("Error: [Login] role not found:", user.Role)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't sign you in"})
	}

	tokenResult, err := auth.GenerateToken(user, role.Scopes)
	if err != nil {
		log.Println("Error: [Login] token generation failed:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't sign you in"})
	}

	return c.JSON(http.StatusOK, httpx.Response{
		Success: true,
		Data:    tokenResult,
	})
}

func Verify(c echo.Context) error {
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}

func Me(c echo.Context) error {
	user, ok := c.Get("user").(store.User)
	if !ok {
		return c.JSON(http.StatusUnauthorized, httpx.Response{Error: "Please sign in again"})
	}
	scopes, _ := c.Get("scopes").([]string)
	data := map[string]any{
		"userId": user.ID,
		"user":   user.DisplayName,
		"role":   user.Role,
		"scopes": scopes,
	}
	if empID, err := store.EmployeeIDForUser(user.ID); err == nil {
		data["employeeId"] = empID
	}
	return c.JSON(http.StatusOK, httpx.Response{
		Success: true,
		Data:    data,
	})
}

func Logout(c echo.Context) error {
	auth := c.Request().Header.Get("Authorization")
	if tokenStr, ok := strings.CutPrefix(auth, "Bearer "); ok {
		t := store.Token{Token: tokenStr}
		t.Delete()
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
