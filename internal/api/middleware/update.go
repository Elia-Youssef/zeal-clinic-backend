package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"clinic-api/internal/api/httpx"
	"clinic-api/internal/updater"

	"github.com/labstack/echo/v4"
)

// UpdateGate returns 503 for /api/* while this node is mid self-update, except
// the status endpoint so the UI can keep polling.
func UpdateGate() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if updater.IsInstalling() {
				p := c.Request().URL.Path
				if strings.HasPrefix(p, "/api/") && p != "/api/update/status" {
					return c.JSON(http.StatusServiceUnavailable, httpx.Response{Error: "Update in progress, please wait"})
				}
			}
			return next(c)
		}
	}
}

// requireKey constant-time compares a shared secret from a header. Used by the
// cloud-only endpoints that sit outside JWT auth.
func requireKey(header, secret string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			got := c.Request().Header.Get(header)
			if secret == "" || subtle.ConstantTimeCompare([]byte(got), []byte(secret)) != 1 {
				return c.JSON(http.StatusUnauthorized, httpx.Response{Error: "Not authorized"})
			}
			return next(c)
		}
	}
}

func RequirePublishSecret(secret string) echo.MiddlewareFunc {
	return requireKey("X-Publish-Secret", secret)
}

func RequireSyncSecret(secret string) echo.MiddlewareFunc {
	return requireKey("X-Sync-Secret", secret)
}
