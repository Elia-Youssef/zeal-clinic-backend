package cloudrestore

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"clinic-api/internal/database"
)

// TestMain gives the database package the fixed key of the throwaway test
// databases, which the snapshots and standalone opens of these tests use.
func TestMain(m *testing.M) {
	if err := database.SetKey(strings.Repeat("0", 62) + "ff"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}
