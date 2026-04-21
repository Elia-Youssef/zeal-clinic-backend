package middleware

import (
	"bytes"
	"clinic-api/internal/database/store"
	"io"
	"log"
	"strings"

	"github.com/labstack/echo/v4"
)

// AuditLogger returns middleware that logs all mutation requests (POST, PUT, DELETE)
// to the audit_log table.
func AuditLogger() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			method := c.Request().Method

			// Only log mutations
			if method != "POST" && method != "PUT" && method != "DELETE" {
				return next(c)
			}

			// Skip auth and health endpoints
			path := c.Request().URL.Path
			if strings.Contains(path, "/auth/") || strings.Contains(path, "/health") {
				return next(c)
			}

			// Read request body for details (re-buffer it)
			var bodyStr string
			if c.Request().Body != nil {
				bodyBytes, err := io.ReadAll(c.Request().Body)
				if err == nil {
					bodyStr = string(bodyBytes)
					// Restore body for downstream handlers
					c.Request().Body = io.NopCloser(bytes.NewReader(bodyBytes))
				}
			}

			// Truncate body to avoid storing huge payloads
			if len(bodyStr) > 2000 {
				bodyStr = bodyStr[:2000] + "...(truncated)"
			}

			// Execute the handler first
			err := next(c)

			// Only log if the request succeeded (2xx)
			status := c.Response().Status
			if status >= 200 && status < 300 {
				// Determine action and entity from path
				action := mapMethodToAction(method)
				entityType, entityID := parseEntityFromPath(path)

				userName := ""
				userRole := ""
				if user, ok := c.Get("user").(store.User); ok {
					userName = user.DisplayName
					userRole = user.Role
				}

				entry := store.AuditLogEntry{
					UserName:   userName,
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

// parseEntityFromPath extracts entity type and ID from API paths like /api/patients/PAT-001
func parseEntityFromPath(path string) (entityType, entityID string) {
	// Remove /api/ prefix
	path = strings.TrimPrefix(path, "/api/")

	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return "unknown", ""
	}

	entityType = parts[0]

	// Handle nested paths like /patients/PAT-001/visits
	if len(parts) >= 2 {
		entityID = parts[1]
	}
	// If there's a deeper path like /visits/xxx/photos, use the sub-entity
	if len(parts) >= 3 {
		entityType = parts[0] + "/" + parts[2]
		if len(parts) >= 4 {
			entityID = parts[3]
		}
	}

	return entityType, entityID
}
