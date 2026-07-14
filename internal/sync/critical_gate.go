package sync

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"

	"clinic-api/internal/buildmode"

	"github.com/labstack/echo/v4"
)

// criticalGate keeps conflict-sensitive cloud writes closed until the local
// peer confirms a complete pull-then-push cycle for the current SSE session.
var criticalGate struct {
	sync.RWMutex
	token string
	ready bool
}

func beginCriticalSession() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	criticalGate.Lock()
	criticalGate.token = token
	criticalGate.ready = false
	criticalGate.Unlock()
	return token, nil
}

func endCriticalSession(token string) {
	criticalGate.Lock()
	if criticalGate.token == token {
		criticalGate.token = ""
		criticalGate.ready = false
	}
	criticalGate.Unlock()
}

func markCriticalSessionReady(token string) bool {
	criticalGate.Lock()
	defer criticalGate.Unlock()
	if token == "" || criticalGate.token != token {
		return false
	}
	criticalGate.ready = true
	return true
}

func failCriticalSession(token string) bool {
	criticalGate.Lock()
	defer criticalGate.Unlock()
	if token == "" || criticalGate.token != token {
		return false
	}
	criticalGate.ready = false
	return true
}

func failCurrentCriticalSession() {
	criticalGate.Lock()
	criticalGate.ready = false
	criticalGate.Unlock()
}

func CriticalWritesReady() bool {
	if !buildmode.Cloud {
		return true
	}
	criticalGate.RLock()
	defer criticalGate.RUnlock()
	return criticalGate.ready
}

// RequireCriticalSync blocks conflict-sensitive writes on cloud builds until
// the connected local peer has completed a pull-then-push cycle.
func RequireCriticalSync(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !CriticalWritesReady() {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{
				"error": "Local clinic is offline or still syncing",
				"code":  "sync_not_ready",
			})
		}
		return next(c)
	}
}
