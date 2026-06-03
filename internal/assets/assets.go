// Package assets exposes binary assets (logo, tray icon) that are shared
// across the backend so they are independent of the frontend build output.
package assets

import _ "embed"

//go:embed icon.ico
var icon []byte

// //go:embed logo.png
// var logo []byte

//go:embed invoice_logo.png
var logo []byte

// Icon returns the Windows tray icon (.ico).
func Icon() []byte { return icon }

// // Logo returns the clinic logo used in generated PDFs (.png).
// func Logo() []byte { return logo }

func InvoiceLogo() []byte { return logo }
