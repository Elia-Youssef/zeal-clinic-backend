package store

import (
	"errors"
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
// against the given columns using LIKE. The filter is split on whitespace into
// tokens: a row must match every token (AND), and each token may match any
// column (OR). This makes multi-word search order-independent, so "jaden smith"
// still finds "Jaden Will Smith" even though the words aren't adjacent.
func (lp ListParams) FilterClause(columns ...string) (string, []any) {
	tokens := strings.Fields(lp.Filter)
	if len(tokens) == 0 {
		return "", nil
	}
	var groups []string
	var args []any
	for _, tok := range tokens {
		// Escape LIKE wildcards so the token matches literally; the backslash
		// escape char is declared per-condition with ESCAPE.
		esc := tok
		esc = strings.ReplaceAll(esc, `\`, `\\`)
		esc = strings.ReplaceAll(esc, "%", `\%`)
		esc = strings.ReplaceAll(esc, "_", `\_`)
		f := "%" + esc + "%"
		conditions := make([]string, 0, len(columns))
		for _, col := range columns {
			conditions = append(conditions, col+` LIKE ? ESCAPE '\'`)
			args = append(args, f)
		}
		groups = append(groups, "("+strings.Join(conditions, " OR ")+")")
	}
	return strings.Join(groups, " AND "), args
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

// requireRow returns ErrNotFound unless table holds a row with id. A write
// whose statements would otherwise touch no row for an unknown id, or fail on
// a foreign key, calls it first so the missing record answers as not found.
func requireRow(q DBTX, table, id string) error {
	var exists int
	if err := q.QueryRow(fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM %s WHERE id = ?)", table), id).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return ErrNotFound
	}
	return nil
}

// requireReference is requireRow for a record the request names in its body
// rather than addresses in its path: a missing one is a fault of the input,
// so it answers ErrValidation "<name> not found" instead of not found.
func requireReference(q DBTX, table, name, id string) error {
	err := requireRow(q, table, id)
	if errors.Is(err, ErrNotFound) {
		return fmt.Errorf("%w: %s not found", ErrValidation, name)
	}
	return err
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
