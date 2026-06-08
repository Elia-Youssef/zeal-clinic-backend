package store

import (
	"strings"

	"github.com/google/uuid"
)

// Both peers derive one id per business key, so concurrent rows merge instead of
// colliding on a secondary UNIQUE. Key must be immutable.
var syncIDNamespace = uuid.MustParse("9f8c7b6a-5d4e-4c3b-a2f1-0e9d8c7b6a5f")

func DeterministicID(parts ...string) string {
	return uuid.NewSHA1(syncIDNamespace, []byte(strings.Join(parts, "\x00"))).String()
}

func balanceID(entityType, entityID, currencyID string) string {
	return DeterministicID("balances", entityType, entityID, currencyID)
}
