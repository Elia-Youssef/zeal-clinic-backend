package middleware

import (
	"bytes"
	"clinic-api/internal/database/store"
	"io"
	"log"
	"regexp"
	"strings"

	"github.com/labstack/echo/v4"
)

var passwordRedactor = regexp.MustCompile(`(?i)("(?:\w*password\w*|pin|secret|token)"\s*:\s*)"[^"]*"`)

func AuditLogger() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			method := c.Request().Method

			if method != "POST" && method != "PUT" && method != "DELETE" {
				return next(c)
			}

			path := c.Request().URL.Path
			if strings.HasPrefix(path, "/api/auth/") || path == "/health" {
				return next(c)
			}

			var bodyStr string
			if c.Request().Body != nil {
				bodyBytes, err := io.ReadAll(c.Request().Body)
				if err == nil {
					bodyStr = string(bodyBytes)
					c.Request().Body = io.NopCloser(bytes.NewReader(bodyBytes))
				}
			}

			bodyStr = passwordRedactor.ReplaceAllString(bodyStr, `$1"[REDACTED]"`)
			if len(bodyStr) > 2000 {
				bodyStr = bodyStr[:2000] + "...(truncated)"
			}

			err := next(c)

			status := c.Response().Status
			if status >= 200 && status < 300 {
				action := mapMethodToAction(method)
				entityType, entityID := parseEntityFromPath(path)

				userID := ""
				userRole := ""
				if user, ok := c.Get("user").(store.User); ok {
					userID = user.ID
					userRole = user.Role
				}

				entry := store.AuditLogEntry{
					UserID:     userID,
					UserRole:   userRole,
					Action:     action,
					EntityType: entityType,
					EntityID:   entityID,
					Details:    bodyStr,
					IPAddress:  c.RealIP(),
				}

				if logErr := entry.Log(); logErr != nil {
					log.Printf("Audit log failed: %v", logErr)
				}
			}

			return err
		}
	}
}

func mapMethodToAction(method string) string {
	switch method {
	case "POST":
		return "create"
	case "PUT":
		return "update"
	case "DELETE":
		return "delete"
	default:
		return "unknown"
	}
}

func parseEntityFromPath(path string) (entityType, entityID string) {
	path = strings.TrimPrefix(path, "/api/")

	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return "unknown", ""
	}

	entityType = parts[0]

	if len(parts) >= 2 {
		entityID = parts[1]
	}
	if len(parts) >= 3 {
		entityType = parts[0] + "/" + parts[2]
		if len(parts) >= 4 {
			entityID = parts[3]
		}
	}

	return entityType, entityID
}
