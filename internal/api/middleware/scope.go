package middleware

import (
	"net/http"
	"slices"

	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"

	"github.com/labstack/echo/v4"
)

// RequireScope checks that the authenticated user (set by AuthMiddleware) has the required scope.
// Scopes are loaded from the user's role row by the auth middleware and placed in context.
func RequireScope(scope string) echo.MiddlewareFunc {
	return RequireAnyScope(scope)
}

// RequireAnyScope passes when the user holds at least one of the given scopes.
func RequireAnyScope(scopes ...string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			held, _ := c.Get("scopes").([]string)
			for _, s := range scopes {
				if slices.Contains(held, s) {
					return next(c)
				}
			}
			return c.JSON(http.StatusForbidden, httpx.Response{Error: "You don't have permission"})
		}
	}
}

// RequireAllScopes passes only when the user holds every one of the given scopes.
func RequireAllScopes(scopes ...string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			held, _ := c.Get("scopes").([]string)
			for _, s := range scopes {
				if !slices.Contains(held, s) {
					return c.JSON(http.StatusForbidden, httpx.Response{Error: "You don't have permission"})
				}
			}
			return next(c)
		}
	}
}

// RequireScopeOrSelf allows the request when the user holds the scope, or when
// the route's :id resolves to the caller's own record (via self), so a user can
// always read their own data even with no scopes.
func RequireScopeOrSelf(scope string, self func(c echo.Context) string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			scopes, _ := c.Get("scopes").([]string)
			if slices.Contains(scopes, scope) {
				return next(c)
			}
			if id := self(c); id != "" && id == c.Param("id") {
				return next(c)
			}
			return c.JSON(http.StatusForbidden, httpx.Response{Error: "You don't have permission"})
		}
	}
}

// SelfUserID returns the caller's own user id.
func SelfUserID(c echo.Context) string {
	u, _ := c.Get("user").(store.User)
	return u.ID
}

// SelfEmployeeID returns the employee id linked to the caller, or "" if none.
func SelfEmployeeID(c echo.Context) string {
	u, _ := c.Get("user").(store.User)
	id, err := store.EmployeeIDForUser(u.ID)
	if err != nil {
		return ""
	}
	return id
}
