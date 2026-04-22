package middleware

import (
	"log"
	"strings"
	"time"

	"clinic-api/internal/api/httpx"
	"clinic-api/internal/auth"
	"clinic-api/internal/database/store"

	"github.com/labstack/echo/v4"
)

func AuthMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		tokenStr := getTokenFromRequest(c)
		if tokenStr == "" {
			log.Println("Error: [Auth] No token provided")
			return c.JSON(401, httpx.Response{Error: "Not Authorized"})
		}

		claims, err := auth.ParseToken(tokenStr)
		if err != nil {
			log.Println("Error: [Auth] Invalid token:", err)
			return c.JSON(401, httpx.Response{Error: "Not Authorized"})
		}

		dbToken := store.Token{}
		if err := dbToken.GetByValue(tokenStr); err != nil {
			log.Println("Error: [Auth] Token not found in database")
			return c.JSON(401, httpx.Response{Error: "Not Authorized"})
		}

		expiresAt, err := dbToken.ExpiresAt.Time()
		if err != nil || expiresAt.Before(time.Now().UTC()) {
			log.Println("Error: [Auth] Token expired")
			t := store.Token{Token: tokenStr}
			t.Delete()
			return c.JSON(401, httpx.Response{Error: "Not Authorized"})
		}

		userID, _ := claims["sub"].(string)

		user := store.User{}
		if err := user.GetByID(userID); err != nil || !user.IsActive {
			log.Println("Error: [Auth] User not found or inactive")
			return c.JSON(401, httpx.Response{Error: "Not Authorized"})
		}

		role := store.Role{}
		if err := role.GetByName(user.Role); err != nil {
			log.Println("Error: [Auth] Role not found:", user.Role)
			return c.JSON(401, httpx.Response{Error: "Not Authorized"})
		}

		c.Set("user", user)
		c.Set("role", user.Role)
		c.Set("scopes", role.Scopes)

		return next(c)
	}
}

func getTokenFromRequest(c echo.Context) string {
	auth := c.Request().Header.Get("Authorization")
	if tokenStr, ok := strings.CutPrefix(auth, "Bearer "); ok {
		return tokenStr
	}
	// Fallback for browser EventSource which cannot set custom headers.
	return c.QueryParam("access_token")
}
