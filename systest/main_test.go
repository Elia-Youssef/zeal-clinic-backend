//go:build systest

// Package systest runs a clinic node and a cloud node side by side on loopback and
// checks replication, the cloud write gate, conflict handling, cloud restore and
// backups over HTTP only. scripts/ci.ps1 -Stage system builds the two binaries and
// runs the suite with their paths; without them every test is skipped.
package systest

import (
	"os"
	"strconv"
	"testing"
	"time"
)

type config struct {
	clinicBin     string
	cloudBin      string
	altCloudBin   string
	altVersion    string
	workDir       string
	syncSecret    string
	publishSecret string
	clinicPort    int
	cloudPort     int
	proxyPort     int
	altCloudPort  int
	full          bool
}

func loadConfig() (config, bool) {
	c := config{
		clinicBin:     os.Getenv("SYSTEST_CLINIC_BIN"),
		cloudBin:      os.Getenv("SYSTEST_CLOUD_BIN"),
		altCloudBin:   os.Getenv("SYSTEST_ALT_CLOUD_BIN"),
		altVersion:    os.Getenv("SYSTEST_ALT_VERSION"),
		workDir:       os.Getenv("SYSTEST_WORK_DIR"),
		syncSecret:    os.Getenv("SYSTEST_SYNC_SECRET"),
		publishSecret: os.Getenv("SYSTEST_PUBLISH_SECRET"),
		clinicPort:    envPort("SYSTEST_CLINIC_PORT", 55581),
		cloudPort:     envPort("SYSTEST_CLOUD_PORT", 55582),
		proxyPort:     envPort("SYSTEST_PROXY_PORT", 55580),
		altCloudPort:  envPort("SYSTEST_ALT_CLOUD_PORT", 55555),
		full:          os.Getenv("SYSTEST_FULL") == "1",
	}
	ok := c.clinicBin != "" && c.cloudBin != "" && c.workDir != "" && c.syncSecret != ""
	return c, ok
}

func envPort(name string, fallback int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil && v > 0 {
		return v
	}
	return fallback
}

func TestMain(m *testing.M) {
	if pid := os.Getenv(breakHelperEnv); pid != "" {
		os.Exit(runBreakHelper(pid))
	}
	os.Exit(m.Run())
}

// TestSystem runs the scenarios in order against one pair of nodes. They share
// state, so the first failure stops the rest.
func TestSystem(t *testing.T) {
	cfg, ok := loadConfig()
	if !ok {
		t.Skip("no node binaries: the suite runs through scripts/ci.ps1 -Stage system")
	}
	h := newHarness(t, cfg)

	steps := []struct {
		name string
		run  func(*testing.T, *harness)
	}{
		{"gate_closed_before_first_connect", stepGateClosedBeforeConnect},
		{"demo_push_refused", stepDemoPushRefused},
		{"initial_sync", stepInitialSync},
		{"write_gate", stepWriteGate},
		{"newer_wins", stepNewerWins},
		{"no_updated_at", stepNoUpdatedAt},
		{"ledger", stepLedger},
		{"deletes", stepDeletes},
		{"version_mismatch", stepVersionMismatch},
		{"update_peer", stepUpdatePeer},
		{"resilience", stepResilience},
		{"backup_drill", stepBackupDrill},
		{"cloud_restore_failures", stepCloudRestoreFailures},
		{"cloud_restore", stepCloudRestore},
		{"long_sse", stepLongSSE},
		{"large_upload", stepLargeUpload},
		{"shutdown", stepShutdown},
	}
	failed := ""
	for _, s := range steps {
		if failed != "" {
			t.Run(s.name, func(t *testing.T) { t.Skipf("not run: %s failed", failed) })
			continue
		}
		start := time.Now()
		if !t.Run(s.name, func(t *testing.T) { s.run(t, h) }) {
			failed = s.name
		}
		t.Logf("step %s: %s", s.name, time.Since(start).Round(100*time.Millisecond))
	}
}
