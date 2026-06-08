// Package updatestate is the self-update recovery record, shared by the app and
// the Windows swapper. It lives in a file (not the DB) so it survives the
// rollback's DB-snapshot restore. Stdlib-only.
package updatestate

import (
	"encoding/json"
	"io"
	"os"
)

type Phase string

const (
	Idle     Phase = ""
	Applying Phase = "applying"
	Trial    Phase = "trial" // linux: booted once, not yet confirmed healthy
	Success  Phase = "success"
	Failed   Phase = "failed"
)

type State struct {
	Phase      Phase  `json:"phase"`
	From       string `json:"from"`
	Target     string `json:"target"`
	AppExe     string `json:"appExe"`
	DBPath     string `json:"dbPath"`
	StagingDir string `json:"stagingDir"`
	BackupExe  string `json:"backupExe"`
	DBSnapshot string `json:"dbSnapshot"`
	ZipPath    string `json:"zipPath"`
	Port       string `json:"port"`
	PID        int    `json:"pid"`
	Error      string `json:"error,omitempty"`
}

// Read reports a missing file as Idle, no error.
func Read(path string) (State, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return State{}, nil
		}
		return State{}, err
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return State{}, err
	}
	return s, nil
}

func Write(path string, s State) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func Clear(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0700)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
