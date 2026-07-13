package legacyimport

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type rowset struct {
	cols []string
	rows [][]any
}

func newRowset(cols ...string) *rowset { return &rowset{cols: cols} }

func (r *rowset) add(vals ...any) {
	if len(vals) != len(r.cols) {
		panic(fmt.Sprintf("rowset: got %d values for %d columns", len(vals), len(r.cols)))
	}
	r.rows = append(r.rows, vals)
}

func (r *rowset) len() int    { return len(r.rows) }
func (r *rowset) empty() bool { return len(r.rows) == 0 }

func (r *rowset) insert(ctx context.Context, tx *sql.Tx, table string) error {
	if len(r.rows) == 0 {
		return nil
	}

	rowsPerBatch := 900 / len(r.cols)
	if rowsPerBatch < 1 {
		rowsPerBatch = 1
	}

	colList := strings.Join(r.cols, ", ")
	tuple := "(" + strings.TrimSuffix(strings.Repeat("?, ", len(r.cols)), ", ") + ")"

	for start := 0; start < len(r.rows); start += rowsPerBatch {
		end := start + rowsPerBatch
		if end > len(r.rows) {
			end = len(r.rows)
		}
		batch := r.rows[start:end]

		placeholders := strings.TrimSuffix(strings.Repeat(tuple+", ", len(batch)), ", ")
		args := make([]any, 0, len(batch)*len(r.cols))
		for _, row := range batch {
			args = append(args, row...)
		}

		stmt := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s", table, colList, placeholders)
		if _, err := tx.ExecContext(ctx, stmt, args...); err != nil {
			return fmt.Errorf("insert into %s: %w", table, err)
		}
	}
	return nil
}
