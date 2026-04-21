package store

import "database/sql"

// ErrNotFound is returned by store methods when a lookup or mutation targets a
// row that does not exist. Aliased to sql.ErrNoRows so errors propagated
// directly from Scan calls satisfy errors.Is checks without extra translation.
var ErrNotFound = sql.ErrNoRows
