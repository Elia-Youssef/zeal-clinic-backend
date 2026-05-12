//go:build !cloud

package systray

import (
	"context"
	_ "embed"
	"log"
	"time"

	"clinic-api/internal/browser"

	trayui "github.com/getlantern/systray"
)

//go:embed icon.ico
var iconData []byte

// Server is what the tray needs to know about the running HTTP server:
// where it's listening and how to shut it down.
type Server interface {
	Port() string
	Shutdown(ctx context.Context) error
}

// Run installs the tray icon and blocks until the user picks Quit
// (or another goroutine calls Quit). Must be called on the main goroutine.
func Run(srv Server) {
	url := "http://localhost:" + srv.Port()

	trayui.Run(
		func() {
			trayui.SetIcon(iconData)
			trayui.SetTitle("Zeal Clinic")
			trayui.SetTooltip("Zeal Clinic — running at " + url)

			mOpen := trayui.AddMenuItem("Open Browser", "Open the app in your browser")
			trayui.AddSeparator()
			mQuit := trayui.AddMenuItem("Quit", "Shut down Zeal Clinic")

			go func() {
				for {
					select {
					case <-mOpen.ClickedCh:
						_ = browser.Open(url)
					case <-mQuit.ClickedCh:
						log.Println("[systray] quit requested")
						trayui.Quit()
						return
					}
				}
			}()
		},
		func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = srv.Shutdown(ctx)
		},
	)
}

// Quit triggers the tray to exit from outside the tray goroutine
// (e.g. when the server dies unexpectedly).
func Quit() { trayui.Quit() }
