//go:build windows

package updater

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// applyUpdate runs the installer silently via ShellExecute "runas" so UAC can
// elevate it (os/exec can't). The installer swaps the files in Program Files and
// relaunches us as the original user via --post-update; we exit so it can
// replace the now-unlocked .exe.
func applyUpdate(installerPath string) error {
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(installerPath)
	params, _ := windows.UTF16PtrFromString(
		"/VERYSILENT /SUPPRESSMSGBOXES /NORESTART /NOCANCEL /CLOSEAPPLICATIONS /RESTARTAPPLICATIONS")

	if err := windows.ShellExecute(0, verb, file, params, nil, 0); err != nil { // 0 == SW_HIDE
		return fmt.Errorf("launch installer: %w", err)
	}

	go func() {
		time.Sleep(750 * time.Millisecond)
		os.Exit(0)
	}()
	return nil
}
