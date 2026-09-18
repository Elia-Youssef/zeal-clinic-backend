package store

import "math"

// Money is stored as float64 (REAL in SQLite) on purpose. Amounts are
// two-decimal currency values, every stored amount passes through Round2, and
// the balance projections are recomputed from the transaction history rather
// than accumulated, so no residue can build up. Moving to integer cents would
// touch every financial table, the sync payloads of both nodes, the legacy
// importer, the PDFs and the dashboard for no gain at a clinic's volumes, so
// the REAL columns stay.

// Round2 normalizes a monetary value to 2 decimal places. Every money value
// written to the DB (transaction amounts, balance projections, invoice totals)
// is funneled through this so floating-point accumulation can't leave
// un-zeroable residue like 0.00000001.
func Round2(v float64) float64 {
	return math.Round(v*100) / 100
}
