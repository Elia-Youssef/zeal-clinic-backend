//go:build cloud

package systray

import "context"

// Server matches the desktop tray's interface so the package compiles on
// cloud builds. Run/Quit are unused; main.go gates them on cloudMode.
type Server interface {
	Port() string
	Shutdown(ctx context.Context) error
}

func Run(srv Server) {}
func Quit()          {}
