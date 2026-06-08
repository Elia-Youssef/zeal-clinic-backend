//go:build cloud

package config

import _ "embed"

//go:embed cloud.env.defaults
var embeddedEnv string
