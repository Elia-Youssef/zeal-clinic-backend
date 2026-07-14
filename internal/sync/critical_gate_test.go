package sync

import "testing"

func TestCriticalGateSessionLifecycle(t *testing.T) {
	token, err := beginCriticalSession()
	if err != nil {
		t.Fatal(err)
	}
	defer endCriticalSession(token)

	if markCriticalSessionReady("stale-token") {
		t.Fatal("stale token opened the gate")
	}
	if !markCriticalSessionReady(token) {
		t.Fatal("current token did not open the gate")
	}
	failCurrentCriticalSession()
	criticalGate.RLock()
	ready := criticalGate.ready
	criticalGate.RUnlock()
	if ready {
		t.Fatal("sync failure did not close the gate")
	}
	if !markCriticalSessionReady(token) {
		t.Fatal("current session could not recover after failure")
	}

	newToken, err := beginCriticalSession()
	if err != nil {
		t.Fatal(err)
	}
	if markCriticalSessionReady(token) {
		t.Fatal("old token opened a newer session")
	}
	endCriticalSession(token)
	if !markCriticalSessionReady(newToken) {
		t.Fatal("old session disconnect closed the newer session")
	}
	endCriticalSession(newToken)
	if markCriticalSessionReady(newToken) {
		t.Fatal("disconnected session was still accepted")
	}
}
