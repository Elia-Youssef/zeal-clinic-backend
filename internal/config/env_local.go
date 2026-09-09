//go:build !cloud

package config

import "embed"

// envFiles holds the clinic build's env files: the committed dev defaults
// (local.env.defaults), the template (local.env.example) and, when it exists
// at build time, the git-ignored local override (local.env).
//
//go:embed local.env.defaults local.env*
var envFiles embed.FS

// envFile is the clinic build's override file; its defaults are envFile + ".defaults".
const envFile = "local.env"
