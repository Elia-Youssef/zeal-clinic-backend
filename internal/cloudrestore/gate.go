package cloudrestore

import (
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/labstack/echo/v4"
)

var errRestoreInProgress = errors.New("cloud restore already in progress")

const releaseRequestKey = "cloudrestore.release-request"

// maintenanceGate rejects new API work and drains finite requests before a
// restore closes the database pools. Long-lived handlers release their slot
// explicitly after authentication and any other database access is complete.
type maintenanceGate struct {
	mu          sync.Mutex
	cond        *sync.Cond
	active      int
	maintenance bool
}

func newMaintenanceGate() *maintenanceGate {
	g := &maintenanceGate{}
	g.cond = sync.NewCond(&g.mu)
	return g
}

func (g *maintenanceGate) middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := c.Request().URL.Path
			if !strings.HasPrefix(path, "/api/") && path != "/files" && !strings.HasPrefix(path, "/files/") {
				return next(c)
			}

			g.mu.Lock()
			if g.maintenance {
				g.mu.Unlock()
				return c.JSON(http.StatusServiceUnavailable, response{Error: "Cloud restore in progress, please wait"})
			}
			tracked := path != "/api/sync/events"
			if tracked {
				g.active++
			}
			g.mu.Unlock()

			if tracked {
				var once sync.Once
				release := func() {
					once.Do(func() {
						g.mu.Lock()
						g.active--
						g.cond.Broadcast()
						g.mu.Unlock()
					})
				}
				c.Set(releaseRequestKey, release)
				defer release()
			}
			return next(c)
		}
	}
}

// begin is called by the restore request after it has passed middleware, so
// one active request (the caller itself) is allowed to remain.
func (g *maintenanceGate) begin() error {
	g.mu.Lock()
	if g.maintenance {
		g.mu.Unlock()
		return errRestoreInProgress
	}
	g.maintenance = true
	g.mu.Unlock()

	g.mu.Lock()
	defer g.mu.Unlock()
	for g.active > 1 {
		g.cond.Wait()
	}
	return nil
}

// MarkLongLivedReady keeps an HTTP stream connected without treating it as
// active database work. Call it only after authentication and setup that may
// access the database have completed.
func MarkLongLivedReady(c echo.Context) {
	if release, ok := c.Get(releaseRequestKey).(func()); ok {
		release()
	}
}

func (g *maintenanceGate) end() {
	g.mu.Lock()
	g.maintenance = false
	g.cond.Broadcast()
	g.mu.Unlock()
}
