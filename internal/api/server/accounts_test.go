package server

import (
	"io"
	"log"
	"net/http"
	"strings"
	"testing"

	"clinic-api/internal/database/store"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// loginAs signs in and returns the bearer token.
func loginAs(t *testing.T, e *echo.Echo, username, password string) string {
	t.Helper()
	rec := doRequest(t, e, http.MethodPost, "/api/auth/login",
		asJSON(t, map[string]string{"username": username, "password": password}), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("login %s: %d %s", username, rec.Code, rec.Body.String())
	}
	var data struct {
		Token string `json:"token"`
	}
	decodeEnvelope(t, rec.Body, &data)
	if data.Token == "" {
		t.Fatalf("login %s: no token in %s", username, rec.Body.String())
	}
	return data.Token
}

// createUserAccount creates a user without a linked employee and returns its id.
// An empty password leaves the account without one.
func createUserAccount(t *testing.T, e *echo.Echo, tok, username, role, password string) string {
	t.Helper()
	body := map[string]any{"username": username, "displayName": "Test " + username, "role": role}
	if password != "" {
		body["password"] = password
	}
	rec := doRequest(t, e, http.MethodPost, "/api/users", asJSON(t, body), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create user %s: %d %s", username, rec.Code, rec.Body.String())
	}
	var u struct {
		ID string `json:"id"`
	}
	decodeEnvelope(t, rec.Body, &u)
	return u.ID
}

// createLinkedEmployee creates an employee with a user account in the given
// role and returns the employee and user ids.
func createLinkedEmployee(t *testing.T, e *echo.Echo, tok, first, last, username, role, password string) (employeeID, userID string) {
	t.Helper()
	body := employeePayload(first, last)
	body["username"] = username
	body["password"] = password
	body["userRole"] = role
	rec := doRequest(t, e, http.MethodPost, "/api/employees", asJSON(t, body), tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create employee %s: %d %s", username, rec.Code, rec.Body.String())
	}
	var emp struct {
		ID     string  `json:"id"`
		UserID *string `json:"userId"`
	}
	decodeEnvelope(t, rec.Body, &emp)
	if emp.ID == "" || emp.UserID == nil || *emp.UserID == "" {
		t.Fatalf("create employee %s: no linked user in %s", username, rec.Body.String())
	}
	return emp.ID, *emp.UserID
}

// roleScopes reads every role's scopes from the roles table, split the way
// the auth middleware splits them.
func roleScopes(t *testing.T) map[string][]string {
	t.Helper()
	rows, err := store.RDB.Query(`SELECT name, scopes FROM roles`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var name, scopes string
		if err := rows.Scan(&name, &scopes); err != nil {
			t.Fatal(err)
		}
		out[name] = []string{}
		if scopes != "" {
			out[name] = strings.Split(scopes, ",")
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// setRoleScopes replaces a role's scopes directly in the table.
func setRoleScopes(t *testing.T, role string, scopes ...string) {
	t.Helper()
	if _, err := store.DB.Exec(`UPDATE roles SET scopes = ? WHERE name = ?`, strings.Join(scopes, ","), role); err != nil {
		t.Fatal(err)
	}
}

// quietServerLogs silences the request log of the servers created after the
// call, and the standard logger, until the test ends.
func quietServerLogs(t *testing.T) {
	t.Helper()
	prevRequestLog := middleware.DefaultLoggerConfig.Output
	prevLog := log.Writer()
	middleware.DefaultLoggerConfig.Output = io.Discard
	log.SetOutput(io.Discard)
	t.Cleanup(func() {
		middleware.DefaultLoggerConfig.Output = prevRequestLog
		log.SetOutput(prevLog)
	})
}
