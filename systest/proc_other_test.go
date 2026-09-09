//go:build systest && !windows

package systest

import (
	"os/exec"
	"syscall"
)

func startInGroup(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd.Start()
}

func interrupt(pid int) error {
	return syscall.Kill(pid, syscall.SIGINT)
}

func runBreakHelper(string) int { return 2 }
