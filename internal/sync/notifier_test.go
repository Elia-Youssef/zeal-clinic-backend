package sync

import (
	"net/http"
	"testing"
)

func TestShouldNotifyWrite_IgnoresSyncProtocol(t *testing.T) {
	for _, path := range []string{
		"/api/sync/pull",
		"/api/sync/push",
		"/api/sync/ready",
		"/api/sync/failed",
	} {
		if shouldNotifyWrite(http.MethodPost, path) {
			t.Errorf("sync request %q scheduled another sync", path)
		}
	}
}

func TestShouldNotifyWrite_MutationsOnly(t *testing.T) {
	if !shouldNotifyWrite(http.MethodPost, "/api/client-invoices") {
		t.Fatal("application mutation did not schedule sync")
	}
	if shouldNotifyWrite(http.MethodGet, "/api/client-invoices") {
		t.Fatal("GET scheduled sync")
	}
}
