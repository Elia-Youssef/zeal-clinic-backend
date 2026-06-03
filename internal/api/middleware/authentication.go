package middleware

import (
	"fmt"
	"log"
	"strings"
	"time"

	"clinic-api/internal/api/httpx"
	"clinic-api/internal/auth"
	"clinic-api/internal/database/store"

	"github.com/labstack/echo/v4"
)

func AuthMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			tokenStr := ""

			authHeader := c.Request().Header.Get("Authorization")
			if t, ok := strings.CutPrefix(authHeader, "Bearer "); ok {
				tokenStr = t
			}

			if tokenStr == "" {
				log.Println("Error: [Auth] No token provided")
				return c.JSON(401, httpx.Response{Error: "Please sign in again"})
			}

			msg, ok := checkAuth(c, tokenStr)
			if !ok {
				log.Println(msg)
				return c.JSON(401, httpx.Response{Error: "Please sign in again"})
			}

			return next(c)
		}
	}
}

func checkAuth(c echo.Context, tokenStr string) (string, bool) {
	claims, err := auth.ParseToken(tokenStr)
	if err != nil {
		return fmt.Sprintf("Error: [Auth] Invalid token: %s", err.Error()), false
	}

	dbToken := store.Token{}
	if err := dbToken.GetByValue(tokenStr); err != nil {
		return "Error: [Auth] Token not found in database", false
	}

	expiresAt, err := dbToken.ExpiresAt.Time()
	if err != nil || expiresAt.Before(time.Now().UTC()) {
		t := store.Token{Token: tokenStr}
		t.Delete()
		return "Error: [Auth] Token expired", false
	}

	userID, _ := claims["sub"].(string)

	user := store.User{}
	if err := user.GetByID(userID); err != nil || !user.IsActive {
		return "Error: [Auth] User not found or inactive", false
	}

	role := store.Role{}
	if err := role.GetByName(user.Role); err != nil {
		return fmt.Sprintf("Error: [Auth] Role not found: %s", user.Role), false
	}

	c.Set("user", user)
	c.Set("role", user.Role)
	c.Set("scopes", role.Scopes)

	return "", true
}
