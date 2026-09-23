package server

import (
	"sort"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

// routeInventory lists the registered routes as sorted "METHOD path" lines.
// The catch-all entries Echo adds for group middleware are left out.
func routeInventory(e *echo.Echo) []string {
	seen := map[string]bool{}
	var lines []string
	for _, r := range e.Routes() {
		if r.Method == echo.RouteNotFound {
			continue
		}
		line := r.Method + " " + r.Path
		if !seen[line] {
			seen[line] = true
			lines = append(lines, line)
		}
	}
	sort.Strings(lines)
	return lines
}

// Adding or removing a route changes the golden list; update it deliberately.
// The cloud list is taken with every machine secret configured, so the sync,
// publish and peer-update routes are mounted.
func TestRouteInventory(t *testing.T) {
	setupTestEnv(t)
	e := CreateServerWithOptions(Options{})
	lines := routeInventory(e)
	t.Logf("%s build: %d routes", buildName(), len(lines))
	checkGolden(t, "testdata/routes/"+buildName()+".txt", strings.Join(lines, "\n")+"\n")
}
