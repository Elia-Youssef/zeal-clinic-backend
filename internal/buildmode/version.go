package buildmode

import "strings"

// Version is the build version, injected at release time via
// -ldflags "-X clinic-api/internal/buildmode.Version=..." from the
// repo-root VERSION file. Defaults to "dev" for un-stamped builds.
var Version = "dev"

// Release reports whether this binary is a stamped release build. Unstamped
// builds ("dev") and debug builds stamped "<version>-dev" (make build, make
// dev) are dev builds.
func Release() bool {
	return IsRelease(Version)
}

// IsRelease reports whether version is the stamp of a release build.
func IsRelease(version string) bool {
	return version != "dev" && !strings.HasSuffix(version, "-dev")
}
