package middleware

import (
	"bytes"
	"clinic-api/internal/database/store"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/labstack/echo/v4"
)

var passwordRedactor = regexp.MustCompile(`(?i)("(?:\w*password\w*|pin|secret|token)"\s*:\s*)"[^"]*"`)

// createdAnswerLimit caps how much of an answer is kept to read the id of the
// record it created; one record's answer is far smaller.
const createdAnswerLimit = 64 << 10

func AuditLogger() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			method := c.Request().Method

			action := mapMethodToAction(method)
			if action == "unknown" {
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

			// When the path names no record, a create's answer names the new one.
			entityType, entityID := parseEntityFromPath(path)
			var answer *answerCapture
			if entityID == "" {
				answer = &answerCapture{ResponseWriter: c.Response().Writer}
				c.Response().Writer = answer
			}

			err := next(c)

			if answer != nil {
				c.Response().Writer = answer.ResponseWriter
			}
			status := c.Response().Status
			if status >= 200 && status < 300 {
				if answer != nil {
					entityID = createdID(answer.body.Bytes())
				}

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

// answerCapture passes an answer through and keeps its first
// createdAnswerLimit bytes.
type answerCapture struct {
	http.ResponseWriter
	body bytes.Buffer
}

func (w *answerCapture) Write(b []byte) (int, error) {
	if room := createdAnswerLimit - w.body.Len(); room > 0 {
		w.body.Write(b[:min(len(b), room)])
	}
	return w.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the writer below (flush, hijack).
func (w *answerCapture) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// createdID returns the id of the record in an answer's envelope
// ({"Data": {"id": ...}}), or "" when it names none.
func createdID(answer []byte) string {
	var envelope struct {
		Data struct {
			ID string `json:"id"`
		}
	}
	if json.Unmarshal(answer, &envelope) != nil {
		return ""
	}
	return envelope.Data.ID
}

func mapMethodToAction(method string) string {
	switch method {
	case "POST":
		return "create"
	case "PUT", "PATCH":
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
