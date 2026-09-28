package store

import (
	"database/sql"
	"errors"
	"fmt"

	"clinic-api/internal/validation"

	sqlite3 "github.com/ncruces/go-sqlite3"
)

// ErrNotFound is returned by store methods when a lookup or mutation targets a
// row that does not exist. Aliased to sql.ErrNoRows so errors propagated
// directly from Scan calls satisfy errors.Is checks without extra translation.
var ErrNotFound = sql.ErrNoRows

var ErrConflict = errors.New("conflict")

// ErrValidation marks invalid input; handlers map it to 400. It aliases the
// validation package's sentinel, so both validation kinds (a wrapped message
// and a field-level validation.Errors map) answer one errors.Is check.
var ErrValidation = validation.Err

// constraintError turns an SQLite constraint failure in err into the store's
// own error kinds, so a write the database refuses for a data reason answers a
// client error instead of a server error. One rule, applied once at the exit
// of every store write that can hit a unique or foreign-key constraint (its
// inserts and its updates of constrained columns; helper rows written inside
// a wrapped parent, notes-only updates that cannot touch a constrained
// column, and deletes guarded by dependency checks stay unwrapped): a unique
// or primary-key clash is ErrConflict carrying the caller's duplicate message
// (empty means the generic one); a foreign-key failure on a referenced parent
// is ErrValidation with one generic message, because the driver does not say
// which parent was missing, so one precise answer is not possible. CHECK and
// NOT NULL failures are deliberately not translated: reaching one means a
// validation is missing, and the 500 shows the bug. Everything else,
// including an already translated error, comes back unchanged.
func constraintError(err error, duplicate string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, sqlite3.CONSTRAINT_UNIQUE), errors.Is(err, sqlite3.CONSTRAINT_PRIMARYKEY):
		if duplicate == "" {
			duplicate = "This record already exists"
		}
		return fmt.Errorf("%w: %s", ErrConflict, duplicate)
	case errors.Is(err, sqlite3.CONSTRAINT_FOREIGNKEY):
		return fmt.Errorf("%w: Related record not found", ErrValidation)
	default:
		return err
	}
}
