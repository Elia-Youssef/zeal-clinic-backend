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
	"sync"

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
	if auth.BlankPassword(r.Password) {
		e["password"] = "password is required"
	}
	if len(e) > 0 {
		return e
	}
	return nil
}

// loginDummyHash is the hash an unknown username or a disabled account is
// checked against, so those refusals run the same argon2id work as a wrong
// password. It is made once, on first use, by auth.HashPassword, so it carries
// the parameters of every freshly stored hash. The attempt is refused whatever
// the comparison says.
var loginDummyHash = sync.OnceValue(func() string {
	hash, err := auth.HashPassword("no account behind this sign-in")
	if err != nil {
		log.Println("Error: [Login] failed to build the comparison hash:", err)
	}
	return hash
})

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
	// The password is compared as typed: it is never trimmed.
	password := req.Password

	user := store.User{}
	found := true
	if err := user.GetByUsername(username); err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			log.Println("Error: [Login] db error:", err)
			return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't sign you in"})
		}
		found = false
	}
	known := found && user.IsActive

	if known && user.PasswordHash == "" {
		// The first sign-in of an account without a password sets it.
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
		// An unknown username, a disabled account and a wrong password get
		// the same status and body, and each runs one argon2id comparison: a
		// wrong password against the stored hash, the other two against
		// loginDummyHash (the parameters of a freshly stored hash). So neither
		// the answer nor the hashing time tells them apart while the stored
		// hash uses the current parameters; only the first unknown-username or
		// disabled-account refusal of a process also builds the dummy hash.
		// Failed attempts are logged without the username, at debug level.
		hash := user.PasswordHash
		reason := "wrong password"
		if !found {
			reason = "unknown username"
		} else if !user.IsActive {
			reason = "disabled account"
		}
		if !known {
			hash = loginDummyHash()
		}
		if match, err := auth.ComparePassword(password, hash); err != nil || !match || !known {
			tracking.Debug(c, "[Login] "+reason)
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
	grantedScopes, _ := c.Get("scopes").([]string)
	data := map[string]any{
		"userId": user.ID,
		"user":   user.DisplayName,
		"role":   user.Role,
		"scopes": grantedScopes,
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
