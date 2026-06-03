package store

import (
	"fmt"
	"log"
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
func (lp ListParams) FilterClause(columns ...string) (string, []any) {
	if lp.Filter == "" {
		return "", nil
	}
	var conditions []string
	var args []any
	// Escape LIKE wildcards so the filter matches literally; the backslash
	// escape char is declared per-condition with ESCAPE.
	esc := lp.Filter
	esc = strings.ReplaceAll(esc, `\`, `\\`)
	esc = strings.ReplaceAll(esc, "%", `\%`)
	esc = strings.ReplaceAll(esc, "_", `\_`)
	f := "%" + esc + "%"
	for _, col := range columns {
		conditions = append(conditions, col+` LIKE ? ESCAPE '\'`)
		args = append(args, f)
	}
	return "(" + strings.Join(conditions, " OR ") + ")", args
}

// DateRangeClause builds a SQL condition fragment restricting column to the
// half-open range [From, To). Bounds are normalized via RangeStart/RangeEnd so
// a bare YYYY-MM-DD includes the whole calendar day. Either bound may be empty;
// returns "" when both are unset. The fragment has no WHERE/AND prefix, so
// callers join it like any other condition.
func (lp ListParams) DateRangeClause(column string) (string, []any) {
	var conditions []string
	var args []any
	if lp.From != "" {
		conditions = append(conditions, column+" >= ?")
		args = append(args, RangeStart(lp.From))
	}
	if lp.To != "" {
		conditions = append(conditions, column+" < ?")
		args = append(args, RangeEnd(lp.To))
	}
	if len(conditions) == 0 {
		return "", nil
	}
	return strings.Join(conditions, " AND "), args
}

// OrderClause returns a SQL " ORDER BY ..." suffix. If Sort is set and matches
// an entry in allowed, it sorts by that column with direction from Order
// (default ASC). Otherwise it falls back to fallback (a raw SQL fragment); if
// fallback is empty no ORDER BY is emitted. The allowed map whitelists sort
// inputs to prevent SQL injection: keys are accepted Sort values, values are
// SQL column expressions.
func (lp ListParams) OrderClause(allowed map[string]string, fallback string) string {
	if lp.Sort != "" {
		if col, ok := allowed[lp.Sort]; ok {
			dir := "ASC"
			if strings.EqualFold(lp.Order, "desc") {
				dir = "DESC"
			}
			return " ORDER BY " + col + " " + dir
		}
	}
	if fallback == "" {
		return ""
	}
	return " ORDER BY " + fallback
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
// Fail-closed: if a query errors (e.g. typo in table name), returns true so
// callers block destructive operations instead of silently allowing them.
func HasDependencies(id string, deps map[string]string) bool {
	for table, column := range deps {
		var count int
		err := RDB.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = ?", table, column), id).Scan(&count)
		if err != nil {
			log.Printf("[store.HasDependencies] %s.%s lookup failed (assuming dependency exists): %v", table, column, err)
			return true
		}
		if count > 0 {
			return true
		}
	}
	return false
}

// NameExists reports whether a row in table already uses name
// (case-insensitive), ignoring the row with id == excludeID (pass "" when
// creating). Used to enforce unique names. Fail-closed like HasDependencies: a
// query error returns true so callers reject the write rather than risk a duplicate.
func NameExists(table, name, excludeID string) bool {
	var count int
	err := RDB.QueryRow(
		fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE name = ? COLLATE NOCASE AND id != ?", table),
		name, excludeID,
	).Scan(&count)
	if err != nil {
		log.Printf("[store.NameExists] %s lookup failed (assuming exists): %v", table, err)
		return true
	}
	return count > 0
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
