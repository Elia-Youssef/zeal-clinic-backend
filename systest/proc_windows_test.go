//go:build systest && windows

package systest

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"unsafe"
)

const (
	ctrlBreakEvent = 1
	createNoWindow = 0x08000000
)

var (
	kernel32                     = syscall.NewLazyDLL("kernel32.dll")
	procGenerateConsoleCtrlEvent = kernel32.NewProc("GenerateConsoleCtrlEvent")
	procGetConsoleProcessList    = kernel32.NewProc("GetConsoleProcessList")
	procAttachConsole            = kernel32.NewProc("AttachConsole")
	procFreeConsole              = kernel32.NewProc("FreeConsole")
	procSetConsoleCtrlHandler    = kernel32.NewProc("SetConsoleCtrlHandler")
)

// startInGroup starts a node in its own process group, so a Ctrl+Break reaches
// only that node. Without a console the node gets none either.
func startInGroup(cmd *exec.Cmd) error {
	flags := uint32(syscall.CREATE_NEW_PROCESS_GROUP)
	if len(consoleProcesses()) == 0 {
		flags |= createNoWindow
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flags}
	return cmd.Start()
}

func consoleProcesses() []uint32 {
	buf := make([]uint32, 4096)
	n, _, _ := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 || int(n) > len(buf) {
		return nil
	}
	return buf[:n]
}

// interrupt sends Ctrl+Break to the node's process group, the graceful stop the
// server handles like Ctrl+C. When the node is on another console, a copy of the
// test binary attaches to that console and sends it from there.
func interrupt(pid int) error {
	for _, p := range consoleProcesses() {
		if int(p) == pid {
			if r, _, err := procGenerateConsoleCtrlEvent.Call(ctrlBreakEvent, uintptr(pid)); r == 0 {
				return fmt.Errorf("GenerateConsoleCtrlEvent: %v", err)
			}
			return nil
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	helper := exec.Command(exe)
	helper.Env = append(os.Environ(), breakHelperEnv+"="+strconv.Itoa(pid))
	helper.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
	if err := helper.Run(); err != nil {
		return fmt.Errorf("break helper: %v", err)
	}
	return nil
}

func runBreakHelper(pidText string) int {
	pid, err := strconv.Atoi(pidText)
	if err != nil {
		return 2
	}
	procFreeConsole.Call()
	if r, _, _ := procAttachConsole.Call(uintptr(pid)); r == 0 {
		return 3
	}
	procSetConsoleCtrlHandler.Call(0, 1)
	if r, _, _ := procGenerateConsoleCtrlEvent.Call(ctrlBreakEvent, uintptr(pid)); r == 0 {
		return 4
	}
	return 0
}
