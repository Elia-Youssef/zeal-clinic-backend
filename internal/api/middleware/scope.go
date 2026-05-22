package middleware

import (
	"net/http"
	"slices"

	"github.com/labstack/echo/v4"
)

// RequireScope checks that the authenticated user (set by AuthMiddleware) has the required scope.
// Scopes are embedded in the JWT token and placed in context by the auth middleware.
func RequireScope(scope string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			scopes, ok := c.Get("scopes").([]string)
			if !ok || len(scopes) == 0 {
				return c.JSON(http.StatusForbidden, map[string]string{
					"error": "no scopes found",
				})
			}

			if slices.Contains(scopes, scope) {
				return next(c)
			}

			return c.JSON(http.StatusForbidden, map[string]string{
				"error": "missing required scope",
			})
		}
	}
}
