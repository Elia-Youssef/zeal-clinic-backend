package models

import "database/sql"

// DB is the package-level database connection used by all model query methods.
var DB *sql.DB

func BoolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
