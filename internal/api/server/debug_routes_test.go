package server

import (
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"
)

func registersRoute(e *echo.Echo, method, path string) bool {
	for _, r := range e.Routes() {
		if r.Method == method && r.Path == path {
			return true
		}
	}
	return false
}

// The test-notification route serves development runs and the automated
// tests; the built server never registers it.
func TestDebugRoutes_OnlyInDevelopmentRuns(t *testing.T) {
	setupTestEnv(t)
	const route = "/api/notifications/test"
	if registersRoute(CreateServerWithOptions(Options{}), http.MethodPost, route) {
		t.Fatal("the built server registers the test notification route")
	}
	if !registersRoute(CreateServerWithOptions(Options{DebugRoutes: true}), http.MethodPost, route) {
		t.Fatal("a development server lacks the test notification route")
	}
}
