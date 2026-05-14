package store

import (
	"database/sql"
)

// DBTX is satisfied by both *sql.DB and *sql.Tx, allowing model helpers
// to run inside or outside an existing transaction.
type DBTX interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// DB is the write-only connection pool (MaxOpenConns=1) used for Exec and Begin.
var DB *sql.DB

// RDB is the read-only connection pool used for Query and QueryRow.
var RDB *sql.DB

// DropdownItem is a lightweight struct for dropdown/select endpoints.
type DropdownItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ListParams holds optional pagination, filtering, and sorting parameters for
// list queries. Sort is the requested sort field (typically a JSON field name
// that each list function maps to a SQL column); Order is "asc" or "desc".
type ListParams struct {
	Offset int
	Limit  int
	Filter string
	Sort   string
	Order  string
}
