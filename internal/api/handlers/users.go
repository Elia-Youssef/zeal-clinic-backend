package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/auth"
	"clinic-api/internal/database/store"
	"clinic-api/internal/realtime"
	"errors"
	"log"
	"net/http"
	"slices"
	"strings"

	"github.com/labstack/echo/v4"
)

// super-admin is seed-only and not grantable via the API.
var assignableRoles = []string{"admin", "staff", "nurse"}

func GetAllUsers(c echo.Context) error {
	params := parseListParams(c)
	users := store.UserList{}
	total, err := users.GetAll(params)
	if err != nil {
		log.Println("Error: [GetAllUsers] failed to fetch users:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load users"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: users, Total: total}})
}

func GetUserByID(c echo.Context) error {
	var item store.User
	if err := item.GetByID(c.Param("id")); errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "User not found"})
	} else if err != nil {
		log.Println("Error: [GetUserByID] failed to fetch user:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load user"})
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
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	body.User.IsActive = true

	if !slices.Contains(assignableRoles, body.User.Role) {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid role"})
	}
	if err := body.User.IsValid(); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
	}
	var hash string
	password := strings.TrimSpace(body.Password)
	if password != "" {
		h, err := auth.HashPassword(password)
		if err != nil {
			log.Println("Error: [CreateUser] failed to hash password:", err)
			return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't create user"})
		}
		hash = h
	}

	if err := body.User.Create(hash); err != nil {
		log.Println("Error: [CreateUser] failed to create user:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't create user"})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: body.User})
}

func UpdateUser(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdateUser] invalid request:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	delete(updates, "id")
	delete(updates, "username")

	// Handle password change separately
	var passwordHash string
	if pw, ok := updates["password"]; ok {
		delete(updates, "password")
		if pwStr, ok := pw.(string); ok {
			pwStr = strings.TrimSpace(pwStr)
			if pwStr != "" {
				h, err := auth.HashPassword(pwStr)
				if err != nil {
					log.Println("Error: [UpdateUser] failed to hash password:", err)
					return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't update user"})
				}
				passwordHash = h
			}
		}
	}

	targetID := c.Param("id")
	caller := c.Get("user").(store.User)

	newRole, roleChanging := updates["role"].(string)
	if roleChanging && !slices.Contains(assignableRoles, newRole) {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid role"})
	}

	deactivating := false
	if active, ok := updates["isActive"].(bool); ok && !active {
		deactivating = true
	}

	if targetID == caller.ID {
		if roleChanging && newRole != caller.Role {
			return c.JSON(http.StatusBadRequest, httpx.Response{Error: "You can't change your own role"})
		}
		if deactivating {
			return c.JSON(http.StatusBadRequest, httpx.Response{Error: "You can't deactivate your own account"})
		}
	}

	var current store.User
	if err := current.GetByID(targetID); errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "User not found"})
	} else if err != nil {
		log.Println("Error: [UpdateUser] failed to load user:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't update user"})
	}

	demoting := roleChanging && current.Role == "admin" && newRole != "admin"
	if current.Role == "admin" && current.IsActive && (demoting || deactivating) {
		count, err := store.CountActiveAdmins()
		if err != nil {
			log.Println("Error: [UpdateUser] failed to count admins:", err)
			return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't update user"})
		}
		if count <= 1 {
			return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Can't remove the last active admin"})
		}
	}

	user := store.User{ID: targetID}
	if err := user.Update(updates); errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "User not found"})
	} else if err != nil {
		log.Println("Error: [UpdateUser] failed to update user:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't update user"})
	}

	if passwordHash != "" {
		if err := user.UpdatePassword(passwordHash); err != nil {
			log.Println("Error: [UpdateUser] failed to update password:", err)
			return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't update password"})
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
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "User not found"})
	} else if err != nil {
		log.Println("Error: [GetUserActions] failed to load user:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load activity"})
	}
	params := parseListParams(c)
	entries := store.AuditLogEntryList{}
	total, err := entries.GetByUserID(user.ID, params)
	if err != nil {
		log.Println("Error: [GetUserActions] failed to fetch actions:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load activity"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: entries, Total: total}})
}
