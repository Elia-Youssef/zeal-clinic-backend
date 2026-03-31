package models

import (
	"database/sql"
	"fmt"
)

// DB is the package-level database connection used by all model query methods.
var DB *sql.DB

func BoolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// HasDependencies checks whether the given id exists in any of the specified
// table/column pairs. The deps map keys are table names and values are the
// column to match against id. Returns true on the first match found.
func HasDependencies(id string, deps map[string]string) bool {
	for table, column := range deps {
		var count int
		err := DB.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = ?", table, column), id).Scan(&count)
		if err != nil {
			continue
		}
		if count > 0 {
			return true
		}
	}
	return false
}

// DeleteDependencies removes all rows that reference the given id in the
// specified table/column pairs. Uses a transaction so either all deletes
// succeed or none do.
func DeleteDependencies(id string, deps map[string]string) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for table, column := range deps {
		if _, err := tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE %s = ?", table, column), id); err != nil {
			return fmt.Errorf("delete from %s: %w", table, err)
		}
	}

	return tx.Commit()
}
