package store

import (
	"database/sql"
	"errors"
)

// ErrNotFound is returned by store methods when a lookup or mutation targets a
// row that does not exist. Aliased to sql.ErrNoRows so errors propagated
// directly from Scan calls satisfy errors.Is checks without extra translation.
var ErrNotFound = sql.ErrNoRows

var ErrConflict = errors.New("conflict")

var ErrValidation = errors.New("validation") // invalid input; handlers map to 400
