package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/auth"
	"clinic-api/internal/database/store"
	"clinic-api/internal/realtime"
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllUsers(c echo.Context) error {
	params := parseListParams(c)
	users := store.UserList{}
	total, err := users.GetAll(params)
	if err != nil {
		log.Println("Error: [GetAllUsers] failed to fetch users:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch users"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: users, Total: total}})
}

func GetUserByID(c echo.Context) error {
	var item store.User
	if err := item.GetByID(c.Param("id")); errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "user not found"})
	} else if err != nil {
		log.Println("Error: [GetUserByID] failed to fetch user:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch user"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: item})
}

func CreateUser(c echo.Context) error {
	var body struct {
		store.User
		Password string `json:"password"`
	}
	if err := c.Bind(&body); err != nil {
		log.Println("Error: [CreateUser] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	body.User.IsActive = true

	if err := body.User.IsValid(); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "validation failed"})
	}
	var hash string
	if body.Password != "" {
		h, err := auth.HashPassword(body.Password)
		if err != nil {
			log.Println("Error: [CreateUser] failed to hash password:", err)
			return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to create user"})
		}
		hash = h
	}

	if err := body.User.Create(hash); err != nil {
		log.Println("Error: [CreateUser] failed to create user:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to create user"})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: body.User})
}

func UpdateUser(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdateUser] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	delete(updates, "id")
	delete(updates, "username")

	// Handle password change separately
	var passwordHash string
	if pw, ok := updates["password"]; ok {
		delete(updates, "password")
		if pwStr, ok := pw.(string); ok && pwStr != "" {
			h, err := auth.HashPassword(pwStr)
			if err != nil {
				log.Println("Error: [UpdateUser] failed to hash password:", err)
				return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to update user"})
			}
			passwordHash = h
		}
	}

	user := store.User{ID: c.Param("id")}
	if err := user.Update(updates); errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "user not found"})
	} else if err != nil {
		log.Println("Error: [UpdateUser] failed to update user:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to update user"})
	}

	if passwordHash != "" {
		if err := user.UpdatePassword(passwordHash); err != nil {
			log.Println("Error: [UpdateUser] failed to update password:", err)
			return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to update password"})
		}
	}

	if _, roleChanged := updates["role"]; roleChanged {
		realtime.SendTo(user.ID, realtime.Event{Type: "scopes_changed"})
	}

	if active, ok := updates["isActive"].(bool); ok && !active {
		realtime.SendTo(user.ID, realtime.Event{Type: "account_disabled"})
		if err := (&store.Token{}).DeleteByUser(user.ID); err != nil {
			log.Println("Error: [UpdateUser] failed to revoke tokens:", err)
		}
	}

	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: user})
}

func GetUserActions(c echo.Context) error {
	var user store.User
	if err := user.GetByID(c.Param("id")); errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "user not found"})
	} else if err != nil {
		log.Println("Error: [GetUserActions] failed to load user:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch actions"})
	}
	params := parseListParams(c)
	entries := store.AuditLogEntryList{}
	total, err := entries.GetByUserID(user.ID, params)
	if err != nil {
		log.Println("Error: [GetUserActions] failed to fetch actions:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch actions"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: entries, Total: total}})
}
