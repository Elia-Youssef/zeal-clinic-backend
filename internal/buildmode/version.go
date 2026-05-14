package buildmode

// Version is the build version, injected at release time via
// -ldflags "-X clinic-api/internal/buildmode.Version=..." from the
// repo-root VERSION file. Defaults to "dev" for un-stamped builds.
var Version = "dev"
