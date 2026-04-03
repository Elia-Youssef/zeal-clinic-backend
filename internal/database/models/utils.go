package models

import (
	"fmt"
	"strings"
)

func BoolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// FilterClause builds a SQL WHERE clause fragment that matches the filter
// string against any of the given columns using LIKE.
func (lp ListParams) FilterClause(columns ...string) (string, []interface{}) {
	if lp.Filter == "" {
		return "", nil
	}
	var conditions []string
	var args []interface{}
	f := "%" + lp.Filter + "%"
	for _, col := range columns {
		conditions = append(conditions, col+" LIKE ?")
		args = append(args, f)
	}
	return "(" + strings.Join(conditions, " OR ") + ")", args
}

// PaginationClause returns a SQL LIMIT/OFFSET suffix.
func (lp ListParams) PaginationClause() string {
	if lp.Limit <= 0 {
		return ""
	}
	s := fmt.Sprintf(" LIMIT %d", lp.Limit)
	if lp.Offset > 0 {
		s += fmt.Sprintf(" OFFSET %d", lp.Offset)
	}
	return s
}

// HasDependencies checks whether the given id exists in any of the specified
// table/column pairs. The deps map keys are table names and values are the
// column to match against id. Returns true on the first match found.
func HasDependencies(id string, deps map[string]string) bool {
	for table, column := range deps {
		var count int
		err := RDB.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = ?", table, column), id).Scan(&count)
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
