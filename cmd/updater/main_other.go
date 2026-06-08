//go:build !windows

// The swapper is Windows-only; Linux self-updates in place. Stub keeps `go build ./...` green.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "ZealUpdater is only used on Windows")
	os.Exit(1)
}
