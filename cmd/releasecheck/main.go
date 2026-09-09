// Command releasecheck is the pre-build check of a release. It refuses unless
// the git-ignored config override of each node to build (internal/config/
// local.env for the clinic, cloud.env for the cloud) exists and holds release
// values, and the two overrides share the sync secret and the DB key. Messages
// name files and keys, never values.
//
//	go run ./cmd/releasecheck [-dir internal/config] clinic|cloud ...
//
// Exit codes: 0 passed, 1 refused, 2 the check could not run. It reads the
// files only: nothing it builds embeds them.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"clinic-api/internal/config/envfile"
)

func main() {
	dir := flag.String("dir", filepath.Join("internal", "config"), "folder with the env files")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/releasecheck [-dir internal/config] clinic|cloud ...")
		flag.PrintDefaults()
	}
	flag.Parse()
	os.Exit(run(*dir, flag.Args()))
}

func run(dir string, nodes []string) int {
	problems, err := envfile.CheckRelease(dir, nodes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "release check: %v\n", err)
		return 2
	}
	if len(problems) > 0 {
		fmt.Fprintf(os.Stderr, "release check refused (%s):\n", strings.Join(nodes, ", "))
		for _, p := range problems {
			fmt.Fprintf(os.Stderr, "  - %s\n", p)
		}
		return 1
	}
	fmt.Printf("release check passed: %s\n", strings.Join(nodes, ", "))
	return 0
}
