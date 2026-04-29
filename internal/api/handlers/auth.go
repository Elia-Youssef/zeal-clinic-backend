package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/auth"
	"clinic-api/internal/database/store"
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
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}

	if err := req.IsValid(); err != nil {
		log.Println("Error: [Login] missing username or password")
		return c.JSON(http.StatusUnauthorized, httpx.Response{Error: "username and password required"})
	}

	username := strings.TrimSpace(req.Username)
	password := strings.TrimSpace(req.Password)

	user := store.User{}
	if err := user.GetByUsername(username); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			log.Println("Error: [Login] user not found:", username)
			return c.JSON(http.StatusUnauthorized, httpx.Response{Error: "invalid username or password"})
		}
		log.Println("Error: [Login] db error:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "authentication failed"})
	}

	if !user.IsActive {
		log.Println("Error: [Login] account disabled:", username)
		return c.JSON(http.StatusForbidden, httpx.Response{Error: "account is disabled"})
	}

	if user.PasswordHash == "" {
		hash, err := auth.HashPassword(password)
		if err != nil {
			log.Println("Error: [Login] failed to hash initial password:", err)
			return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "authentication failed"})
		}
		if err := user.UpdatePassword(hash); err != nil {
			log.Println("Error: [Login] failed to set initial password:", err)
			return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "authentication failed"})
		}
		user.PasswordHash = hash
	} else {
		match, err := auth.ComparePassword(password, user.PasswordHash)
		if err != nil || !match {
			log.Println("Error: [Login] invalid password for:", username)
			return c.JSON(http.StatusUnauthorized, httpx.Response{Error: "invalid username or password"})
		}
	}

	role := store.Role{}
	if err := role.GetByName(user.Role); err != nil {
		log.Println("Error: [Login] role not found:", user.Role)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to resolve user role"})
	}

	tokenResult, err := auth.GenerateToken(user, role.Scopes)
	if err != nil {
		log.Println("Error: [Login] token generation failed:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to generate token"})
	}

	return c.JSON(http.StatusOK, httpx.Response{
		Success: true,
		Data:    tokenResult,
	})
}

func Verify(c echo.Context) error {
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}

func Logout(c echo.Context) error {
	auth := c.Request().Header.Get("Authorization")
	if tokenStr, ok := strings.CutPrefix(auth, "Bearer "); ok {
		t := store.Token{Token: tokenStr}
		t.Delete()
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
