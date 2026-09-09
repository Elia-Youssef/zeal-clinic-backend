//go:build cloud

package config

import "embed"

// envFiles holds the cloud build's env files: the committed dev defaults
// (cloud.env.defaults), the template (cloud.env.example) and, when it exists
// at build time, the git-ignored local override (cloud.env).
//
//go:embed cloud.env.defaults cloud.env*
var envFiles embed.FS

// envFile is the cloud build's override file; its defaults are envFile + ".defaults".
const envFile = "cloud.env"
