//go:build !cloud

package sync_test

import (
	"testing"

	syncpkg "clinic-api/internal/sync"

	"github.com/labstack/echo/v4"
)

// The clinic never serves the machine routes, even with a sync secret.
func TestSyncAPI_NotMountedOnClinicBuilds(t *testing.T) {
	e := echo.New()
	(&syncpkg.API{DB: newNode(t), Secret: "test-sync-secret"}).RegisterRoutes(e)
	if routes := e.Routes(); len(routes) != 0 {
		t.Errorf("clinic build mounted %d routes: %+v", len(routes), routes)
	}
}
