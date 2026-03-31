package middleware

import (
	"log"
	"strings"
	"time"

	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"

	"github.com/labstack/echo/v4"
)

func AuthMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		tokenStr := getTokenFromRequest(c)
		if tokenStr == "" {
			log.Println("Error: [Auth] No token provided")
			return c.JSON(401, utils.Response{Error: "Not Authorized"})
		}

		claims, err := utils.ParseToken(tokenStr)
		if err != nil {
			log.Println("Error: [Auth] Invalid token:", err)
			return c.JSON(401, utils.Response{Error: "Not Authorized"})
		}

		dbToken := models.Token{}
		if err := dbToken.GetByValue(tokenStr); err != nil {
			log.Println("Error: [Auth] Token not found in database")
			return c.JSON(401, utils.Response{Error: "Not Authorized"})
		}

		expiresAt, err := dbToken.ExpiresAt.Time()
		if err != nil || expiresAt.Before(time.Now().UTC()) {
			log.Println("Error: [Auth] Token expired")
			t := models.Token{Token: tokenStr}
			t.Delete()
			return c.JSON(401, utils.Response{Error: "Not Authorized"})
		}

		userID, _ := claims["sub"].(string)

		user := models.User{}
		if err := user.GetByID(userID); err != nil || !user.IsActive {
			log.Println("Error: [Auth] User not found or inactive")
			return c.JSON(401, utils.Response{Error: "Not Authorized"})
		}

		role := models.Role{}
		if err := role.GetByName(user.Role); err != nil {
			log.Println("Error: [Auth] Role not found:", user.Role)
			return c.JSON(401, utils.Response{Error: "Not Authorized"})
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
	return ""
}
