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

func Run(shutdown func(ctx context.Context) error) {
	trayui.Run(
		func() {
			trayui.SetIcon(iconData)
			trayui.SetTitle("Zeal Clinic")

			mOpen := trayui.AddMenuItem("Open Browser", "Open the app in your browser")
			trayui.AddSeparator()
			mQuit := trayui.AddMenuItem("Quit", "Shut down Zeal Clinic")

			go func() {
				for {
					select {
					case <-mOpen.ClickedCh:
						_ = browser.Open()
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
			shutdown(ctx)
		},
	)
}

// Quit triggers the tray to exit from outside the tray goroutine
// (e.g. when the server dies unexpectedly).
func Quit() { trayui.Quit() }
