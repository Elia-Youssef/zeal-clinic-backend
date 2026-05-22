package updater

import "testing"

func TestPostUpdateArgsAlwaysIncludesFlag(t *testing.T) {
	if args := postUpdateArgs(); len(args) == 0 || args[len(args)-1] != "--post-update" {
		// os.Args during tests has no --post-update, so it must be appended.
		t.Errorf("postUpdateArgs() = %v, want trailing --post-update", args)
	}
}
