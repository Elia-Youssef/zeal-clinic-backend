//go:build !cloud

package config

import _ "embed"

//go:embed local.env.defaults
var embeddedEnv string
