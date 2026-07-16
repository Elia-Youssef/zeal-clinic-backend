package updater

import (
	"path/filepath"
	"testing"
	"time"

	"clinic-api/internal/updater/updatestate"
)

func TestPostUpdateArgsAlwaysIncludesFlag(t *testing.T) {
	if args := postUpdateArgs(); len(args) == 0 || args[len(args)-1] != "--post-update" {
		// os.Args during tests has no --post-update, so it must be appended.
		t.Errorf("postUpdateArgs() = %v, want trailing --post-update", args)
	}
}

func TestFinalizeOnBootGatesAPIDuringPostUpdateHealthCheck(t *testing.T) {
	installing.Store(false)
	t.Cleanup(func() { installing.Store(false) })

	stateFile := filepath.Join(t.TempDir(), "state.json")
	st := updatestate.State{Phase: updatestate.Applying, Target: "next"}
	if err := updatestate.Write(stateFile, st); err != nil {
		t.Fatal(err)
	}

	finalizeOnBoot(stateFile, true)
	if !IsInstalling() {
		t.Fatal("post-update health check should keep the API gate closed")
	}

	st.Phase = updatestate.Success
	if err := updatestate.Write(stateFile, st); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for IsInstalling() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if IsInstalling() {
		t.Fatal("successful health check should reopen the API gate")
	}
}

func TestFinalizeOnBootGatesAPIDuringLinuxTrial(t *testing.T) {
	installing.Store(false)
	t.Cleanup(func() { installing.Store(false) })

	stateFile := filepath.Join(t.TempDir(), "state.json")
	if err := updatestate.Write(stateFile, updatestate.State{Phase: updatestate.Trial}); err != nil {
		t.Fatal(err)
	}

	finalizeOnBoot(stateFile, true)
	if !IsInstalling() {
		t.Fatal("trial health check should keep the API gate closed")
	}
}

func TestPostUpdateGateRequiresExplicitHealthCheckResult(t *testing.T) {
	installing.Store(false)
	t.Cleanup(func() { installing.Store(false) })

	stateFile := filepath.Join(t.TempDir(), "state.json")
	st := updatestate.State{Phase: updatestate.Applying, Target: "next"}
	if err := updatestate.Write(stateFile, st); err != nil {
		t.Fatal(err)
	}
	finalizeOnBoot(stateFile, true)

	if err := updatestate.Clear(stateFile); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	if !IsInstalling() {
		t.Fatal("missing state must not reopen APIs without a health-check result")
	}

	st.Phase = updatestate.Failed
	if err := updatestate.Write(stateFile, st); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for IsInstalling() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if IsInstalling() {
		t.Fatal("failed health check should reopen the rolled-back process APIs")
	}
}
