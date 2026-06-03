package store

import "math"

// Round2 normalizes a monetary value to 2 decimal places. Every money value
// written to the DB (transaction amounts, balance projections, invoice totals)
// is funneled through this so floating-point accumulation can't leave
// un-zeroable residue like 0.00000001.
func Round2(v float64) float64 {
	return math.Round(v*100) / 100
}
