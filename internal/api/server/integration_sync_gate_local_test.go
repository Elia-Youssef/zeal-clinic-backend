//go:build !cloud

package server

import "testing"

// openCriticalSyncGate does nothing on clinic builds: the critical-sync gate
// exists only on the cloud, where a connected clinic opens it.
func openCriticalSyncGate(t *testing.T) (closeGate func()) {
	t.Helper()
	return func() {}
}
